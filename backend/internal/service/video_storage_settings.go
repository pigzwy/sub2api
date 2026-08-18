package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const settingKeyVideoStorageConfig = "video_storage_config"

// ErrVideoStorageIncomplete indicates an enabled target with missing credentials.
var ErrVideoStorageIncomplete = errors.New("video storage is enabled but bucket/access_key_id/secret_access_key are incomplete")

// Classified TestConnection failures. Constructing an S3 client never contacts
// the bucket, so a probe is the only thing that can tell these apart; the admin
// UI maps each one to an actionable message instead of a raw SDK string.
var (
	ErrVideoStorageBucketNotFound = errors.New("video storage bucket does not exist at this endpoint")
	ErrVideoStorageAccessDenied   = errors.New("video storage credentials are invalid or lack permission on this bucket")
	ErrVideoStorageUnreachable    = errors.New("video storage endpoint is unreachable")

	// ErrVideoStorageSecretUnreadable means the stored secret failed to decrypt.
	// The ciphertext is never passed to S3 as a signing key: that only produced
	// opaque signature errors far away from the real cause.
	ErrVideoStorageSecretUnreadable = errors.New("stored video storage secret cannot be decrypted")

	// ErrVideoStorageNotProbeable guards against reporting success when the
	// storage client cannot be verified, which is the bug this probe replaces.
	ErrVideoStorageNotProbeable = errors.New("video storage client does not support connection probing")
)

const (
	// videoStorageProbeTimeout bounds the whole probe. A black-holed endpoint
	// must fail fast rather than hang the admin settings page.
	videoStorageProbeTimeout = 8 * time.Second

	// videoStorageFailureLogInterval throttles resolve failures. Failures are no
	// longer cached, so resolve() retries on every request while the problem
	// lasts; without throttling one broken bucket would flood the log.
	videoStorageFailureLogInterval = time.Minute
)

// VideoStorageProbe is implemented by object stores that can verify a bucket end
// to end. Errors wrap ErrVideoStorageBucketNotFound, ErrVideoStorageAccessDenied
// or ErrVideoStorageUnreachable whenever the cause can be classified.
type VideoStorageProbe interface {
	ProbeVideoStorage(ctx context.Context, prefix string) error
}

// videoSecretOrigin tells toConfig whether the secret it received is ciphertext
// read from storage or plaintext typed by an admin / loaded from config.yaml.
// Only stored ciphertext may be decrypted.
type videoSecretOrigin int

const (
	videoSecretFromStore videoSecretOrigin = iota
	videoSecretPlaintext
)

// VideoStorageFactory builds a streaming video store from independent settings.
type VideoStorageFactory func(ctx context.Context, cfg *config.VideoStorageConfig) (VideoObjectStorage, error)

// VideoStorageSettings is the independent admin-managed object storage for
// completed Grok videos. It intentionally does not inherit the image switch,
// bucket, prefix, or credentials.
type VideoStorageSettings struct {
	Enabled       bool `json:"enabled"`
	ReuseBackupS3 bool `json:"reuse_backup_s3"`

	Bucket           string `json:"bucket"`
	Prefix           string `json:"prefix"`
	PresignExpiry    int    `json:"presign_expiry_hours"`
	MaxDownloadBytes int64  `json:"max_download_bytes"`

	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key,omitempty"` //nolint:revive // field name follows AWS convention
	ForcePathStyle  bool   `json:"force_path_style"`
}

// VideoStorageSettingService persists, masks, and hot-reloads video storage settings.
type VideoStorageSettingService struct {
	settingRepo SettingRepository
	encryptor   SecretEncryptor
	backup      *BackupService
	factory     VideoStorageFactory
	fallback    config.VideoStorageConfig

	now func() time.Time

	mu       sync.Mutex
	resolved bool
	storage  VideoObjectStorage
	options  VideoStorageOptions
	enabled  bool

	// Guarded by mu, used only for throttling repeated failure logs.
	lastFailureKind string
	lastFailureAt   time.Time
}

// NewVideoStorageSettingService creates an independent video storage settings service.
func NewVideoStorageSettingService(
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	backup *BackupService,
	factory VideoStorageFactory,
	fallback config.VideoStorageConfig,
) *VideoStorageSettingService {
	service := &VideoStorageSettingService{
		settingRepo: settingRepo,
		encryptor:   encryptor,
		backup:      backup,
		factory:     factory,
		fallback:    fallback,
		now:         time.Now,
	}
	if backup != nil {
		backup.RegisterS3ConfigInvalidator(service.Invalidate)
	}
	return service
}

// Resolver returns the hot-reloadable binding consumed by VideoOffloadService.
func (s *VideoStorageSettingService) Resolver() VideoStorageResolver {
	return func() (VideoObjectStorage, VideoStorageOptions, bool) {
		return s.resolve()
	}
}

func (s *VideoStorageSettingService) resolve() (VideoObjectStorage, VideoStorageOptions, bool) {
	if s == nil {
		return nil, VideoStorageOptions{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resolved {
		return s.storage, s.options, s.enabled
	}

	// Only a settled answer is cached. Caching failures used to pin
	// enabled=false for the rest of the process lifetime, so a transient
	// database or endpoint problem stayed "off" until the next restart even
	// after the underlying cause was gone.
	s.storage, s.options, s.enabled = nil, VideoStorageOptions{}, false
	cfg, err := s.effectiveConfig(context.Background())
	if err != nil {
		if s.shouldLogFailure("settings_load") {
			logger.L().Warn("video_storage.settings_load_failed; video offload stays disabled", zap.Error(err))
		}
		return nil, VideoStorageOptions{}, false
	}
	if !cfg.Enabled {
		// Deliberately off is an answer, not a failure: Update and backup
		// credential changes both invalidate this cache.
		s.resolved = true
		return nil, VideoStorageOptions{}, false
	}
	if !cfg.IsConfigured() {
		if s.shouldLogFailure("incomplete") {
			logger.L().Warn("video_storage is enabled but not fully configured; video offload stays disabled",
				zap.Strings("missing_keys", cfg.MissingCredentialKeys()))
		}
		return nil, VideoStorageOptions{}, false
	}
	storage, err := s.factory(context.Background(), cfg)
	if err != nil {
		if s.shouldLogFailure("client_build") {
			logger.L().Error("video_storage.client_build_failed; video offload stays disabled", zap.Error(err))
		}
		return nil, VideoStorageOptions{}, false
	}
	maxBytes := cfg.MaxDownloadByte
	if maxBytes <= 0 {
		maxBytes = defaultVideoMaxDownloadBytes
	}
	s.storage = storage
	s.options = VideoStorageOptions{Prefix: cfg.Prefix, MaxDownloadBytes: maxBytes}
	s.enabled = true
	s.resolved = true
	s.lastFailureKind, s.lastFailureAt = "", time.Time{}
	return s.storage, s.options, true
}

// shouldLogFailure reports whether this failure deserves a log line. Callers
// hold mu. A different failure kind always logs, so a configuration problem
// turning into an endpoint problem is never hidden by the throttle.
func (s *VideoStorageSettingService) shouldLogFailure(kind string) bool {
	at := s.clock()
	if kind == s.lastFailureKind && at.Sub(s.lastFailureAt) < videoStorageFailureLogInterval {
		return false
	}
	s.lastFailureKind, s.lastFailureAt = kind, at
	return true
}

func (s *VideoStorageSettingService) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// Invalidate drops the cached client so the next request uses current settings.
func (s *VideoStorageSettingService) Invalidate() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.resolved = false
	s.storage = nil
	s.options = VideoStorageOptions{}
	s.enabled = false
	// Let the next failure log immediately: an admin who just saved needs to
	// see whether the new settings still fail.
	s.lastFailureKind, s.lastFailureAt = "", time.Time{}
	s.mu.Unlock()
}

// Get returns masked settings for the admin UI.
func (s *VideoStorageSettingService) Get(ctx context.Context) (*VideoStorageSettings, error) {
	settings, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = videoSettingsFromConfig(s.fallback)
	}
	settings.SecretAccessKey = ""
	return settings, nil
}

// SecretConfigured reports whether a reusable secret is available.
func (s *VideoStorageSettingService) SecretConfigured(ctx context.Context) bool {
	settings, err := s.load(ctx)
	if err != nil || settings == nil {
		return s.fallback.SecretAccessKey != ""
	}
	if settings.ReuseBackupS3 {
		cfg, cfgErr := s.backupCredentials(ctx)
		return cfgErr == nil && cfg != nil && cfg.SecretAccessKey != ""
	}
	return settings.SecretAccessKey != ""
}

// Update persists settings and makes them effective immediately.
func (s *VideoStorageSettingService) Update(ctx context.Context, in VideoStorageSettings) (*VideoStorageSettings, error) {
	normalizeVideoStorageSettings(&in)
	if in.ReuseBackupS3 {
		in.Endpoint, in.Region, in.AccessKeyID, in.SecretAccessKey = "", "", "", ""
		in.ForcePathStyle = false
	} else {
		secretNeedsEncryption := in.SecretAccessKey != ""
		if in.SecretAccessKey == "" {
			if old, err := s.load(ctx); err == nil && old != nil {
				in.SecretAccessKey = old.SecretAccessKey
			} else if s.fallback.SecretAccessKey != "" {
				in.SecretAccessKey = s.fallback.SecretAccessKey
				secretNeedsEncryption = true
			}
		}
		if secretNeedsEncryption {
			if s.backup == nil || !s.backup.EncryptionKeyConfigured() {
				return nil, ErrSecretEncryptionKeyNotConfigured
			}
			encrypted, err := s.encryptor.Encrypt(in.SecretAccessKey)
			if err != nil {
				return nil, fmt.Errorf("encrypt secret: %w", err)
			}
			in.SecretAccessKey = encrypted
		}
	}

	data, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal video storage settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyVideoStorageConfig, string(data)); err != nil {
		return nil, fmt.Errorf("save video storage settings: %w", err)
	}
	s.Invalidate()
	in.SecretAccessKey = ""
	return &in, nil
}

// TestConnection proves the submitted target actually works: it heads the
// bucket, writes a small object under the configured prefix, then deletes it.
// Building a client validates nothing, so the previous build-only check reported
// success for a wrong bucket name, bad credentials, or an unreachable endpoint.
func (s *VideoStorageSettingService) TestConnection(ctx context.Context, in VideoStorageSettings) error {
	normalizeVideoStorageSettings(&in)
	// A secret in the request body is whatever the admin just typed.
	origin := videoSecretPlaintext
	if !in.ReuseBackupS3 && in.SecretAccessKey == "" {
		old, err := s.load(ctx)
		if err == nil && old != nil {
			in.SecretAccessKey = old.SecretAccessKey
			origin = videoSecretFromStore
		} else if s.fallback.SecretAccessKey != "" {
			in.SecretAccessKey = s.fallback.SecretAccessKey
		}
	}
	cfg, err := s.toConfig(ctx, &in, origin)
	if err != nil {
		return err
	}
	if !cfg.IsConfigured() {
		return ErrVideoStorageIncomplete
	}

	probeCtx, cancel := context.WithTimeout(ctx, videoStorageProbeTimeout)
	defer cancel()
	storage, err := s.factory(probeCtx, cfg)
	if err != nil {
		return err
	}
	prober, ok := storage.(VideoStorageProbe)
	if !ok {
		return ErrVideoStorageNotProbeable
	}
	return prober.ProbeVideoStorage(probeCtx, cfg.Prefix)
}

func (s *VideoStorageSettingService) effectiveConfig(ctx context.Context) (*config.VideoStorageConfig, error) {
	settings, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		// config.yaml holds a plaintext secret and never went through Update.
		fallback := s.fallback
		return &fallback, nil
	}
	return s.toConfig(ctx, settings, videoSecretFromStore)
}

func (s *VideoStorageSettingService) toConfig(
	ctx context.Context,
	in *VideoStorageSettings,
	origin videoSecretOrigin,
) (*config.VideoStorageConfig, error) {
	cfg := &config.VideoStorageConfig{
		Enabled:         in.Enabled,
		Bucket:          in.Bucket,
		Prefix:          in.Prefix,
		PresignExpiry:   in.PresignExpiry,
		MaxDownloadByte: in.MaxDownloadBytes,
		Endpoint:        in.Endpoint,
		Region:          in.Region,
		AccessKeyID:     in.AccessKeyID,
		SecretAccessKey: in.SecretAccessKey,
		ForcePathStyle:  in.ForcePathStyle,
	}
	if in.ReuseBackupS3 {
		backupCfg, err := s.backupCredentials(ctx)
		if err != nil {
			return nil, err
		}
		if backupCfg == nil {
			return nil, errors.New("video storage is set to reuse the backup S3 configuration, but no backup S3 configuration exists")
		}
		cfg.Endpoint = backupCfg.Endpoint
		cfg.Region = backupCfg.Region
		cfg.AccessKeyID = backupCfg.AccessKeyID
		cfg.SecretAccessKey = backupCfg.SecretAccessKey
		cfg.ForcePathStyle = backupCfg.ForcePathStyle
		if cfg.Bucket == "" {
			cfg.Bucket = backupCfg.Bucket
		}
	} else if cfg.SecretAccessKey != "" && origin == videoSecretFromStore {
		// Update always encrypts before persisting, so a stored secret that
		// will not decrypt is a real fault (wrong or rotated encryption key),
		// not legacy plaintext. Signing with the ciphertext only turned it into
		// an unexplained SignatureDoesNotMatch much later.
		decrypted, err := s.encryptor.Decrypt(cfg.SecretAccessKey)
		if err != nil {
			logger.L().Error("video_storage.secret_decrypt_failed; refusing to sign with the stored ciphertext",
				zap.Error(err))
			return nil, fmt.Errorf("%w: %w", ErrVideoStorageSecretUnreadable, err)
		}
		cfg.SecretAccessKey = decrypted
	}
	return cfg, nil
}

func (s *VideoStorageSettingService) backupCredentials(ctx context.Context) (*BackupS3Config, error) {
	if s.backup == nil {
		return nil, errors.New("backup service is unavailable")
	}
	return s.backup.loadS3Config(ctx)
}

func (s *VideoStorageSettingService) load(ctx context.Context) (*VideoStorageSettings, error) {
	if s.settingRepo == nil {
		return nil, nil //nolint:nilnil // no repository means no stored settings
	}
	raw, err := s.settingRepo.GetValue(ctx, settingKeyVideoStorageConfig)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, nil //nolint:nilnil // never configured is a valid state
	}
	var settings VideoStorageSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, fmt.Errorf("parse video storage settings: %w", err)
	}
	return &settings, nil
}

func videoSettingsFromConfig(cfg config.VideoStorageConfig) *VideoStorageSettings {
	return &VideoStorageSettings{
		Enabled:          cfg.Enabled,
		Bucket:           cfg.Bucket,
		Prefix:           cfg.Prefix,
		PresignExpiry:    cfg.PresignExpiry,
		MaxDownloadBytes: cfg.MaxDownloadByte,
		Endpoint:         cfg.Endpoint,
		Region:           cfg.Region,
		AccessKeyID:      cfg.AccessKeyID,
		SecretAccessKey:  cfg.SecretAccessKey,
		ForcePathStyle:   cfg.ForcePathStyle,
	}
}

func normalizeVideoStorageSettings(in *VideoStorageSettings) {
	in.Bucket = strings.TrimSpace(in.Bucket)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.Region = strings.TrimSpace(in.Region)
	in.AccessKeyID = strings.TrimSpace(in.AccessKeyID)
	in.SecretAccessKey = strings.TrimSpace(in.SecretAccessKey)
	in.Prefix = strings.Trim(strings.TrimSpace(in.Prefix), "/")
	if in.Prefix == "" {
		in.Prefix = "videos"
	}
	in.Prefix += "/"
	if in.Region == "" {
		in.Region = "auto"
	}
	if in.PresignExpiry <= 0 {
		in.PresignExpiry = 24
	}
	if in.MaxDownloadBytes <= 0 {
		in.MaxDownloadBytes = defaultVideoMaxDownloadBytes
	}
}
