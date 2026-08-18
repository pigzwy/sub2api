//go:build unit

package service

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type savedAudioObject struct {
	key         string
	contentType string
	data        []byte
}

// fakeAudioStorage records every Save. When block is set, Save records first and
// only then waits, so a test can hold uploads in flight and still observe them.
type fakeAudioStorage struct {
	mu    sync.Mutex
	saved []savedAudioObject
	err   error
	block chan struct{}
}

func (f *fakeAudioStorage) Save(_ context.Context, key, contentType string, data []byte) (string, error) {
	f.mu.Lock()
	f.saved = append(f.saved, savedAudioObject{
		key:         key,
		contentType: contentType,
		data:        append([]byte(nil), data...),
	})
	block, err := f.block, f.err
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	return "https://cdn.example/" + key, err
}

func (f *fakeAudioStorage) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.saved)
}

func (f *fakeAudioStorage) all() []savedAudioObject {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]savedAudioObject(nil), f.saved...)
}

var audioOffloadTestClock = time.Date(2026, 8, 18, 9, 30, 0, 0, time.UTC)

func newAudioOffloadFixture(storage AudioObjectStorage, prefix string, enabled bool) *AudioOffloadService {
	svc := NewAudioOffloadService(func() (AudioObjectStorage, AudioStorageOptions, bool) {
		return storage, AudioStorageOptions{Prefix: prefix}, enabled
	})
	svc.now = func() time.Time { return audioOffloadTestClock }
	return svc
}

func requireNoAudioSaved(t *testing.T, storage *fakeAudioStorage) {
	t.Helper()
	require.Never(t, func() bool { return storage.count() > 0 }, 150*time.Millisecond, 15*time.Millisecond)
}

func TestAudioOffloadSkipsSubmissionWhenDisabled(t *testing.T) {
	storage := &fakeAudioStorage{}
	newAudioOffloadFixture(storage, "audio/", false).
		Submit(context.Background(), "req-1", "audio/mpeg", []byte("bytes"))
	requireNoAudioSaved(t, storage)
}

func TestAudioOffloadSkipsEmptyPayload(t *testing.T) {
	storage := &fakeAudioStorage{}
	newAudioOffloadFixture(storage, "audio/", true).
		Submit(context.Background(), "req-1", "audio/mpeg", nil)
	requireNoAudioSaved(t, storage)
}

func TestAudioOffloadStoresDatedKeyDerivedFromContentType(t *testing.T) {
	storage := &fakeAudioStorage{}
	newAudioOffloadFixture(storage, "archive/tts/", true).
		Submit(context.Background(), "tts-req-1", "audio/mpeg; charset=binary", []byte("audio-bytes"))

	require.Eventually(t, func() bool { return storage.count() == 1 }, time.Second, 10*time.Millisecond)
	saved := storage.all()[0]
	require.Equal(t, "archive/tts/2026/08/18/tts-req-1.mp3", saved.key)
	require.Equal(t, "audio/mpeg", saved.contentType)
	require.Equal(t, "audio-bytes", string(saved.data))
}

func TestAudioOffloadFallsBackToGeneratedRequestID(t *testing.T) {
	storage := &fakeAudioStorage{}
	newAudioOffloadFixture(storage, "audio/", true).
		Submit(context.Background(), "   ", "audio/wav", []byte("bytes"))

	require.Eventually(t, func() bool { return storage.count() == 1 }, time.Second, 10*time.Millisecond)
	require.Regexp(t,
		regexp.MustCompile(`^audio/2026/08/18/[0-9a-fA-F-]{36}\.wav$`),
		storage.all()[0].key)
}

func TestAudioOffloadSwallowsStorageFailures(t *testing.T) {
	storage := &fakeAudioStorage{err: errors.New("bucket exploded")}
	// Submit has no error return by design: a failed archive must never reach
	// the caller, which has already written the TTS response.
	newAudioOffloadFixture(storage, "audio/", true).
		Submit(context.Background(), "req-1", "audio/mpeg", []byte("bytes"))

	require.Eventually(t, func() bool { return storage.count() == 1 }, time.Second, 10*time.Millisecond)
	require.Never(t, func() bool { return storage.count() > 1 }, 150*time.Millisecond, 15*time.Millisecond)
}

func TestAudioOffloadDropsSubmissionsWhenSaturated(t *testing.T) {
	storage := &fakeAudioStorage{block: make(chan struct{})}
	svc := newAudioOffloadFixture(storage, "audio/", true)

	// Slots are claimed synchronously inside Submit, so the overflow is
	// deterministic regardless of goroutine scheduling.
	started := time.Now()
	for i := range audioOffloadMaxInFlight + 3 {
		svc.Submit(context.Background(), "req-"+string(rune('a'+i)), "audio/mpeg", []byte("bytes"))
	}
	require.Less(t, time.Since(started), time.Second, "Submit must not block on saturated workers")

	require.Eventually(t, func() bool { return storage.count() == audioOffloadMaxInFlight },
		time.Second, 10*time.Millisecond)
	require.Never(t, func() bool { return storage.count() > audioOffloadMaxInFlight },
		150*time.Millisecond, 15*time.Millisecond)
	close(storage.block)
}

func TestBuildAudioOffloadKeyCannotEscapeThePrefix(t *testing.T) {
	key, err := buildAudioOffloadKey("audio/", "../../etc/passwd", "audio/mpeg", audioOffloadTestClock)
	require.NoError(t, err)
	require.Equal(t, "audio/2026/08/18/..%2F..%2Fetc%2Fpasswd.mp3", key)

	_, err = buildAudioOffloadKey("audio/", "  ", "audio/mpeg", audioOffloadTestClock)
	require.Error(t, err)
}

func TestAudioOffloadExtensionMapping(t *testing.T) {
	for contentType, want := range map[string]string{
		"audio/mpeg":              "mp3",
		"audio/wav":               "wav",
		"audio/aac":               "aac",
		"audio/ogg":               "ogg",
		"audio/flac":              "flac",
		"AUDIO/MP4":               "m4a",
		"audio/opus":              "opus",
		"application/json":        "bin",
		"":                        "bin",
		"audio/mpeg; codecs=mp4a": "mp3",
	} {
		require.Equal(t, want, audioOffloadExtension(contentType), "content type %q", contentType)
	}
	require.Equal(t, "application/octet-stream", audioOffloadMediaType(""))
	require.Equal(t, "audio/mpeg", audioOffloadMediaType("audio/mpeg; charset=binary"))
}
