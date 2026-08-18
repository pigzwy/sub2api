package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// audioOffloadMaxInFlight caps concurrent archive uploads. This is a side
	// channel: the TTS response is already written and already billed when a
	// submission arrives, so dropping a recording is strictly better than
	// applying back pressure to the hot path.
	audioOffloadMaxInFlight = 4

	// audioOffloadTimeout bounds one upload after the request context is
	// detached, so a stalled bucket cannot leak goroutines forever.
	audioOffloadTimeout = 60 * time.Second

	// audioOffloadFailureLogInterval throttles repeated identical failures. A
	// bucket outage would otherwise emit one line per TTS call.
	audioOffloadFailureLogInterval = time.Minute

	defaultAudioOffloadExtension = "bin"
	defaultAudioOffloadMediaType = "application/octet-stream"
)

// AudioObjectStorage stores a fully buffered audio response. The method set
// matches ImageStorage.Save so the media client (*repository.S3ImageStorage)
// serves images, videos, and audio without a second client.
type AudioObjectStorage interface {
	Save(ctx context.Context, key, contentType string, data []byte) (url string, err error)
}

// AudioStorageOptions is the audio half of the media storage settings.
type AudioStorageOptions struct {
	Prefix string
}

// AudioStorageResolver reads the live admin setting on every submission, so
// flipping the switch in the console takes effect without a restart.
type AudioStorageResolver func() (AudioObjectStorage, AudioStorageOptions, bool)

// AudioOffloadService archives completed TTS audio to object storage.
//
// Submission is deliberately fire-and-forget: every failure mode (disabled,
// queue full, upload error, panic) degrades to a log line and leaves the
// client response and the usage record untouched.
type AudioOffloadService struct {
	resolve AudioStorageResolver
	slots   chan struct{}
	now     func() time.Time

	mu              sync.Mutex
	lastFailureKind string
	lastFailureAt   time.Time
}

func NewAudioOffloadService(resolve AudioStorageResolver) *AudioOffloadService {
	return &AudioOffloadService{
		resolve: resolve,
		slots:   make(chan struct{}, audioOffloadMaxInFlight),
		now:     time.Now,
	}
}

// Submit archives one completed TTS response without blocking the caller.
// requestID is the upstream request id; an empty one falls back to a UUID so
// the object still lands under a unique key.
func (s *AudioOffloadService) Submit(ctx context.Context, requestID, contentType string, data []byte) {
	if s == nil || s.resolve == nil || len(data) == 0 {
		return
	}
	storage, options, enabled := s.resolve()
	if !enabled || storage == nil {
		return
	}
	if strings.TrimSpace(requestID) == "" {
		requestID = uuid.NewString()
	}
	key, err := buildAudioOffloadKey(options.Prefix, requestID, contentType, s.clock())
	if err != nil {
		s.logFailure("build_key", requestID, err)
		return
	}

	select {
	case s.slots <- struct{}{}:
	default:
		s.logFailure("queue_full", requestID,
			fmt.Errorf("audio offload queue is full (%d in flight)", audioOffloadMaxInFlight))
		return
	}

	// The client has already been served, so a disconnect must not abort the
	// archive; detach the cancellation but keep the request-scoped values.
	uploadCtx := context.Background()
	if ctx != nil {
		uploadCtx = context.WithoutCancel(ctx)
	}
	mediaType := audioOffloadMediaType(contentType)
	go func() {
		defer func() {
			<-s.slots
			if r := recover(); r != nil {
				s.logFailure("panic", requestID, fmt.Errorf("audio offload panicked: %v", r))
			}
		}()
		runCtx, cancel := context.WithTimeout(uploadCtx, audioOffloadTimeout)
		defer cancel()
		if _, saveErr := storage.Save(runCtx, key, mediaType, data); saveErr != nil {
			s.logFailure("upload", requestID, saveErr)
		}
	}()
}

func (s *AudioOffloadService) logFailure(kind, requestID string, err error) {
	if !s.shouldLogFailure(kind) {
		return
	}
	logger.L().Error("audio_task.offload_failed",
		zap.String("reason", kind),
		zap.String("request_id", strings.TrimSpace(requestID)),
		zap.Error(err))
}

// shouldLogFailure reports whether this failure deserves a log line. A
// different failure kind always logs, so an outage turning into a
// misconfiguration is never hidden by the throttle.
func (s *AudioOffloadService) shouldLogFailure(kind string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := s.clock()
	if kind == s.lastFailureKind && at.Sub(s.lastFailureAt) < audioOffloadFailureLogInterval {
		return false
	}
	s.lastFailureKind, s.lastFailureAt = kind, at
	return true
}

func (s *AudioOffloadService) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// buildAudioOffloadKey lays audio out as <prefix>yyyy/mm/dd/<request id>.<ext>.
// The date segments keep a busy bucket browsable and make lifecycle rules easy.
func buildAudioOffloadKey(prefix, requestID, contentType string, at time.Time) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", errors.New("build audio object key: request ID is empty")
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix != "" {
		prefix += "/"
	}
	// PathEscape keeps the request ID recognizable while preventing a crafted
	// ID from escaping the configured audio prefix.
	safeID := strings.ReplaceAll(url.PathEscape(requestID), "/", "%2F")
	return prefix + at.UTC().Format("2006/01/02") + "/" + safeID + "." + audioOffloadExtension(contentType), nil
}

// audioOffloadExtension maps the upstream media type to a file extension so the
// stored object opens in a player instead of downloading as an unknown blob.
func audioOffloadExtension(contentType string) string {
	switch audioOffloadBaseMediaType(contentType) {
	case "audio/mpeg", "audio/mp3", "audio/x-mpeg":
		return "mp3"
	case "audio/wav", "audio/wave", "audio/x-wav", "audio/vnd.wave":
		return "wav"
	case "audio/aac", "audio/aacp", "audio/x-aac":
		return "aac"
	case "audio/ogg", "application/ogg", "audio/vorbis":
		return "ogg"
	case "audio/flac", "audio/x-flac":
		return "flac"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return "m4a"
	case "audio/opus":
		return "opus"
	case "audio/webm":
		return "webm"
	default:
		return defaultAudioOffloadExtension
	}
}

func audioOffloadMediaType(contentType string) string {
	if base := audioOffloadBaseMediaType(contentType); base != "" {
		return base
	}
	return defaultAudioOffloadMediaType
}

func audioOffloadBaseMediaType(contentType string) string {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.IndexByte(mediaType, ';'); idx >= 0 {
		mediaType = strings.TrimSpace(mediaType[:idx])
	}
	return mediaType
}
