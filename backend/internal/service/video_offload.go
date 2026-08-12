package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultVideoMaxDownloadBytes    int64 = 512 << 20 // 512 MiB
	defaultVideoOffloadLockTTL            = 10 * time.Minute
	defaultVideoOffloadRecordTTL          = 24 * time.Hour
	defaultVideoOffloadExecutionTTL       = 9 * time.Minute
)

var ErrVideoOffloadTooLarge = errors.New("video exceeds the configured download limit")

// VideoObjectStorage is the streaming object-storage contract used by completed
// Grok video tasks. It is intentionally separate from ImageStorage so image-only
// implementations and tests keep their existing byte-slice contract unchanged.
type VideoObjectStorage interface {
	UploadVideo(ctx context.Context, key, contentType string, body io.Reader) error
	PresignVideo(ctx context.Context, key string) (rawURL string, expiresAt time.Time, err error)
}

// VideoOffloadRecord is the compact Redis value for an uploaded video. URLs are
// deliberately not persisted: every status/content request receives a fresh
// presigned URL using the currently configured expiry.
type VideoOffloadRecord struct {
	S3Key      string `json:"s3_key"`
	UploadedAt int64  `json:"uploaded_at"`
}

type VideoOffloadStore interface {
	TryLock(ctx context.Context, requestID string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, requestID string) (*VideoOffloadRecord, error)
	Save(ctx context.Context, requestID string, record *VideoOffloadRecord, ttl time.Duration) error
}

type VideoStorageOptions struct {
	Prefix           string
	MaxDownloadBytes int64
}

// VideoStorageResolver resolves the live admin setting on every operation. The
// settings service caches the concrete client and invalidates it after updates.
type VideoStorageResolver func() (storage VideoObjectStorage, options VideoStorageOptions, enabled bool)

type VideoOffloadLink struct {
	URL       string
	ExpiresAt time.Time
}

type VideoOffloadSource func(ctx context.Context) (*http.Response, error)

type VideoOffloadService struct {
	store      VideoOffloadStore
	resolve    VideoStorageResolver
	lockTTL    time.Duration
	recordTTL  time.Duration
	offloadTTL time.Duration
	now        func() time.Time
}

func NewVideoOffloadService(store VideoOffloadStore, resolve VideoStorageResolver) *VideoOffloadService {
	return newVideoOffloadServiceWithOptions(
		store,
		resolve,
		defaultVideoOffloadLockTTL,
		defaultVideoOffloadRecordTTL,
		defaultVideoOffloadExecutionTTL,
		time.Now,
	)
}

func newVideoOffloadServiceWithOptions(
	store VideoOffloadStore,
	resolve VideoStorageResolver,
	lockTTL, recordTTL, offloadTTL time.Duration,
	now func() time.Time,
) *VideoOffloadService {
	if lockTTL <= 0 {
		lockTTL = defaultVideoOffloadLockTTL
	}
	if recordTTL <= 0 {
		recordTTL = defaultVideoOffloadRecordTTL
	}
	if offloadTTL <= 0 {
		offloadTTL = defaultVideoOffloadExecutionTTL
	}
	// The transfer must stop before the Redis lease expires. This keeps a later
	// status poll from winning an expired lock while the first multipart upload
	// is still active.
	if offloadTTL >= lockTTL {
		offloadTTL = lockTTL - time.Second
		if offloadTTL <= 0 {
			offloadTTL = lockTTL / 2
		}
	}
	if now == nil {
		now = time.Now
	}
	return &VideoOffloadService{
		store:      store,
		resolve:    resolve,
		lockTTL:    lockTTL,
		recordTTL:  recordTTL,
		offloadTTL: offloadTTL,
		now:        now,
	}
}

// Resolve returns a newly signed link when an offload record already exists.
// A nil link and nil error is a normal cache miss or disabled configuration.
func (s *VideoOffloadService) Resolve(ctx context.Context, requestID string) (*VideoOffloadLink, error) {
	storage, _, enabled := s.current()
	if !enabled {
		return nil, nil
	}
	record, err := s.store.Get(ctx, strings.TrimSpace(requestID))
	if err != nil || record == nil {
		return nil, err
	}
	return presignVideoOffload(ctx, storage, record)
}

// ResolveOrOffload returns an existing stored video or atomically claims the
// first completed-status transfer. Lock losers immediately return a cache miss
// so their status response can keep the original passthrough behavior.
func (s *VideoOffloadService) ResolveOrOffload(
	ctx context.Context,
	requestID string,
	source VideoOffloadSource,
) (*VideoOffloadLink, error) {
	storage, options, enabled := s.current()
	if !enabled {
		return nil, nil
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, errors.New("video offload request ID is empty")
	}
	if source == nil {
		return nil, errors.New("video offload source is unavailable")
	}

	record, err := s.store.Get(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("load video offload record: %w", err)
	}
	if record != nil {
		return presignVideoOffload(ctx, storage, record)
	}

	won, err := s.store.TryLock(ctx, requestID, s.lockTTL)
	if err != nil {
		return nil, fmt.Errorf("claim video offload lock: %w", err)
	}
	if !won {
		return nil, nil
	}

	// Recheck after winning in case a previous upload completed between the
	// initial lookup and lock acquisition (for example immediately after expiry).
	record, err = s.store.Get(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("reload video offload record: %w", err)
	}
	if record != nil {
		return presignVideoOffload(ctx, storage, record)
	}

	offloadCtx := context.Background()
	if ctx != nil {
		offloadCtx = context.WithoutCancel(ctx)
	}
	offloadCtx, cancel := context.WithTimeout(offloadCtx, s.offloadTTL)
	defer cancel()

	resp, err := source(offloadCtx)
	if err != nil {
		return nil, fmt.Errorf("download completed video: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("download completed video: empty response")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download completed video: unexpected status %d", resp.StatusCode)
	}

	maxBytes := options.MaxDownloadBytes
	if maxBytes <= 0 {
		maxBytes = defaultVideoMaxDownloadBytes
	}
	if contentLength := videoResponseContentLength(resp); contentLength > maxBytes {
		return nil, fmt.Errorf("%w: content length %d exceeds %d bytes", ErrVideoOffloadTooLarge, contentLength, maxBytes)
	}

	key, err := buildVideoOffloadKey(options.Prefix, requestID)
	if err != nil {
		return nil, err
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "video/mp4"
	}
	body := &videoMaxBytesReader{reader: resp.Body, remaining: maxBytes, limit: maxBytes}
	if err := storage.UploadVideo(offloadCtx, key, contentType, body); err != nil {
		return nil, fmt.Errorf("upload completed video to object storage: %w", err)
	}

	record = &VideoOffloadRecord{
		S3Key:      key,
		UploadedAt: s.now().UTC().UnixMilli(),
	}
	if err := s.store.Save(offloadCtx, requestID, record, s.recordTTL); err != nil {
		return nil, fmt.Errorf("save video offload record: %w", err)
	}
	return presignVideoOffload(offloadCtx, storage, record)
}

func (s *VideoOffloadService) current() (VideoObjectStorage, VideoStorageOptions, bool) {
	if s == nil || s.store == nil || s.resolve == nil {
		return nil, VideoStorageOptions{}, false
	}
	storage, options, enabled := s.resolve()
	if !enabled || storage == nil {
		return nil, VideoStorageOptions{}, false
	}
	if options.MaxDownloadBytes <= 0 {
		options.MaxDownloadBytes = defaultVideoMaxDownloadBytes
	}
	return storage, options, true
}

func presignVideoOffload(ctx context.Context, storage VideoObjectStorage, record *VideoOffloadRecord) (*VideoOffloadLink, error) {
	if storage == nil || record == nil || strings.TrimSpace(record.S3Key) == "" {
		return nil, errors.New("video offload record is incomplete")
	}
	rawURL, expiresAt, err := storage.PresignVideo(ctx, record.S3Key)
	if err != nil {
		return nil, fmt.Errorf("presign stored video: %w", err)
	}
	if strings.TrimSpace(rawURL) == "" || expiresAt.IsZero() {
		return nil, errors.New("presign stored video: empty URL or expiry")
	}
	if strings.ContainsAny(rawURL, "\r\n") {
		return nil, errors.New("presign stored video: URL contains control characters")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("presign stored video: URL must be an HTTPS origin without userinfo")
	}
	return &VideoOffloadLink{URL: rawURL, ExpiresAt: expiresAt}, nil
}

func buildVideoOffloadKey(prefix, requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", errors.New("build video object key: request ID is empty")
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix != "" {
		prefix += "/"
	}
	// PathEscape keeps the request ID recognizable while preventing a crafted ID
	// from escaping the dedicated videos/ sub-prefix.
	safeID := strings.ReplaceAll(url.PathEscape(requestID), "/", "%2F")
	return prefix + "videos/" + safeID + ".mp4", nil
}

func videoResponseContentLength(resp *http.Response) int64 {
	if resp == nil {
		return -1
	}
	raw := strings.TrimSpace(resp.Header.Get("Content-Length"))
	if raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && n >= 0 {
			return n
		}
	}
	if resp.ContentLength >= 0 {
		return resp.ContentLength
	}
	return -1
}

// videoMaxBytesReader never hands more than limit bytes to the multipart
// uploader. Once the limit is consumed it probes one byte: EOF means an exact-
// limit video; any data returns ErrVideoOffloadTooLarge and aborts the upload.
type videoMaxBytesReader struct {
	reader    io.Reader
	remaining int64
	limit     int64
}

func (r *videoMaxBytesReader) Read(p []byte) (int, error) {
	if r == nil || r.reader == nil {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining > 0 {
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}
		n, err := r.reader.Read(p)
		r.remaining -= int64(n)
		return n, err
	}
	var probe [1]byte
	n, err := r.reader.Read(probe[:])
	if n > 0 {
		return 0, fmt.Errorf("%w: streamed body exceeds %d bytes", ErrVideoOffloadTooLarge, r.limit)
	}
	return 0, err
}
