//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type memoryVideoOffloadStore struct {
	mu      sync.Mutex
	locked  map[string]bool
	records map[string]*VideoOffloadRecord
}

func newMemoryVideoOffloadStore() *memoryVideoOffloadStore {
	return &memoryVideoOffloadStore{locked: map[string]bool{}, records: map[string]*VideoOffloadRecord{}}
}

func (s *memoryVideoOffloadStore) TryLock(_ context.Context, requestID string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked[requestID] {
		return false, nil
	}
	s.locked[requestID] = true
	return true, nil
}

func (s *memoryVideoOffloadStore) Get(_ context.Context, requestID string) (*VideoOffloadRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[requestID]
	if record == nil {
		return nil, nil
	}
	copy := *record
	return &copy, nil
}

func (s *memoryVideoOffloadStore) Save(_ context.Context, requestID string, record *VideoOffloadRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *record
	s.records[requestID] = &copy
	return nil
}

type memoryVideoStorage struct {
	mu          sync.Mutex
	uploads     int
	body        []byte
	contentType string
	uploadErr   error
	expiresAt   time.Time
	presignURL  string
}

func (s *memoryVideoStorage) UploadVideo(_ context.Context, _ string, contentType string, body io.Reader) error {
	s.mu.Lock()
	s.uploads++
	s.contentType = contentType
	s.mu.Unlock()
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.body = data
	uploadErr := s.uploadErr
	s.mu.Unlock()
	return uploadErr
}

func (s *memoryVideoStorage) PresignVideo(_ context.Context, key string) (string, time.Time, error) {
	if s.presignURL != "" {
		return s.presignURL, s.expiresAt, nil
	}
	return "https://s3.example.test/" + key + "?signed=1", s.expiresAt, nil
}

func TestVideoOffloadExecutionTimeoutStaysInsideLockLease(t *testing.T) {
	svc := newVideoOffloadServiceWithOptions(
		newMemoryVideoOffloadStore(),
		func() (VideoObjectStorage, VideoStorageOptions, bool) {
			return &memoryVideoStorage{}, VideoStorageOptions{}, true
		},
		time.Minute,
		time.Hour,
		time.Minute,
		time.Now,
	)

	require.Equal(t, time.Minute, svc.lockTTL)
	require.Equal(t, time.Minute-time.Second, svc.offloadTTL)
}

func TestVideoOffloadRejectsUnsafePresignedURL(t *testing.T) {
	for _, rawURL := range []string{
		"http://s3.example.test/video.mp4",
		"https://user@s3.example.test/video.mp4",
		"https://s3.example.test/video.mp4\r\nX-Injected: yes",
	} {
		t.Run(rawURL, func(t *testing.T) {
			storage := &memoryVideoStorage{presignURL: rawURL, expiresAt: time.Now().Add(time.Hour)}
			_, err := presignVideoOffload(t.Context(), storage, &VideoOffloadRecord{S3Key: "videos/task.mp4"})
			require.ErrorContains(t, err, "presign stored video")
		})
	}
}

func videoOffloadTestService(store VideoOffloadStore, storage VideoObjectStorage, maxBytes int64) *VideoOffloadService {
	return newVideoOffloadServiceWithOptions(
		store,
		func() (VideoObjectStorage, VideoStorageOptions, bool) {
			return storage, VideoStorageOptions{Prefix: "images/", MaxDownloadBytes: maxBytes}, true
		},
		time.Minute,
		time.Hour,
		time.Minute,
		func() time.Time { return time.Unix(1700000000, 0) },
	)
}

func TestVideoOffloadConcurrentCompletedOnlyUploadsOnce(t *testing.T) {
	store := newMemoryVideoOffloadStore()
	storage := &memoryVideoStorage{expiresAt: time.Unix(1700003600, 0)}
	svc := videoOffloadTestService(store, storage, 1024)

	started := make(chan struct{})
	release := make(chan struct{})
	var sourceCalls atomic.Int32
	source := func(context.Context) (*http.Response, error) {
		if sourceCalls.Add(1) == 1 {
			close(started)
		}
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"video/mp4"}},
			Body:       io.NopCloser(bytes.NewReader([]byte("video"))),
		}, nil
	}

	var wg sync.WaitGroup
	results := make([]*VideoOffloadLink, 2)
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results[0], errs[0] = svc.ResolveOrOffload(t.Context(), "task-1", source)
	}()
	<-started
	secondDone := make(chan struct{})
	go func() {
		defer wg.Done()
		results[1], errs[1] = svc.ResolveOrOffload(t.Context(), "task-1", source)
		close(secondDone)
	}()
	<-secondDone
	close(release)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, int32(1), sourceCalls.Load())
	require.Equal(t, 1, storage.uploads)
	require.NotNil(t, results[0])
	require.Nil(t, results[1], "lock loser must keep passthrough behavior")
}

func TestVideoOffloadRejectsContentLengthBeforeUpload(t *testing.T) {
	store := newMemoryVideoOffloadStore()
	storage := &memoryVideoStorage{expiresAt: time.Now().Add(time.Hour)}
	svc := videoOffloadTestService(store, storage, 4)

	_, err := svc.ResolveOrOffload(t.Context(), "task-large", func(context.Context) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":   []string{"video/webm"},
				"Content-Length": []string{"5"},
			},
			Body: io.NopCloser(bytes.NewReader([]byte("12345"))),
		}, nil
	})

	require.ErrorIs(t, err, ErrVideoOffloadTooLarge)
	require.Zero(t, storage.uploads)
}

func TestVideoOffloadAbortsStreamingBodyOverLimit(t *testing.T) {
	store := newMemoryVideoOffloadStore()
	storage := &memoryVideoStorage{expiresAt: time.Now().Add(time.Hour)}
	svc := videoOffloadTestService(store, storage, 4)

	_, err := svc.ResolveOrOffload(t.Context(), "task-stream-large", func(context.Context) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{},
			Body:          io.NopCloser(bytes.NewReader([]byte("12345"))),
			ContentLength: -1,
		}, nil
	})

	require.ErrorIs(t, err, ErrVideoOffloadTooLarge)
	require.Equal(t, 1, storage.uploads)
	require.Empty(t, storage.body)
	require.Equal(t, "video/mp4", storage.contentType)
}

func TestVideoOffloadUploadFailureDoesNotPersistRecord(t *testing.T) {
	store := newMemoryVideoOffloadStore()
	storage := &memoryVideoStorage{uploadErr: errors.New("s3 unavailable"), expiresAt: time.Now().Add(time.Hour)}
	svc := videoOffloadTestService(store, storage, 1024)

	_, err := svc.ResolveOrOffload(t.Context(), "task-fail", func(context.Context) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewReader([]byte("video"))),
		}, nil
	})

	require.ErrorContains(t, err, "s3 unavailable")
	record, getErr := store.Get(t.Context(), "task-fail")
	require.NoError(t, getErr)
	require.Nil(t, record)
}
