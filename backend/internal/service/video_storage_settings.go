package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const settingKeyVideoStorageConfig = "video_storage_config"

// ErrVideoStorageIncomplete indicates an enabled target with missing credentials.
var ErrVideoStorageIncomplete = errors.New("video storage is enabled but bucket/access_key_id/secret_access_key are incomplete")

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

	mu       sync.Mutex
	resolved bool
	storage  VideoObjectStorage
	options  VideoStorageOptions
	enabled  bool
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

	s.resolved = true
	s.storage, s.options, s.enabled = nil, VideoStorageOptions{}, false
	cfg, err := s.effectiveConfig(context.Background())
	if err != nil {
		logger.L().Warn("video_storage.settings_load_failed; video offload stays disabled", zap.Error(err))
		return nil, VideoStorageOptions{}, false
	}
	if !cfg.Enabled {
		return nil, VideoStorageOptions{}, false
	}
	if !cfg.IsConfigured() {
		logger.L().Warn("video_storage is enabled but not fully configured; video offload stays disabled",
			zap.Strings("missing_keys", cfg.MissingCredentialKeys()))
		return nil, VideoStorageOptions{}, false
	}
	storage, err := s.factory(context.Background(), cfg)
	if err != nil {
		logger.L().Error("video_storage.client_build_failed; video offload stays disabled", zap.Error(err))
		return nil, VideoStorageOptions{}, false
	}
	maxBytes := cfg.MaxDownloadByte
	if maxBytes <= 0 {
		maxBytes = defaultVideoMaxDownloadBytes
	}
	s.storage = storage
	s.options = VideoStorageOptions{Prefix: cfg.Prefix, MaxDownloadBytes: maxBytes}
	s.enabled = true
	return s.storage, s.options, true
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

// TestConnection validates that the submitted target can build an S3 client.
func (s *VideoStorageSettingService) TestConnection(ctx context.Context, in VideoStorageSettings) error {
	normalizeVideoStorageSettings(&in)
	if !in.ReuseBackupS3 && in.SecretAccessKey == "" {
		old, err := s.load(ctx)
		if err == nil && old != nil {
			in.SecretAccessKey = old.SecretAccessKey
		} else if s.fallback.SecretAccessKey != "" {
			in.SecretAccessKey = s.fallback.SecretAccessKey
		}
	}
	cfg, err := s.toConfig(ctx, &in)
	if err != nil {
		return err
	}
	if !cfg.IsConfigured() {
		return ErrVideoStorageIncomplete
	}
	_, err = s.factory(ctx, cfg)
	return err
}

func (s *VideoStorageSettingService) effectiveConfig(ctx context.Context) (*config.VideoStorageConfig, error) {
	settings, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		fallback := s.fallback
		return &fallback, nil
	}
	return s.toConfig(ctx, settings)
}

func (s *VideoStorageSettingService) toConfig(ctx context.Context, in *VideoStorageSettings) (*config.VideoStorageConfig, error) {
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
	} else if cfg.SecretAccessKey != "" {
		decrypted, err := s.encryptor.Decrypt(cfg.SecretAccessKey)
		if err != nil {
			logger.L().Warn("video_storage secret decrypt failed; treating the stored value as plaintext", zap.Error(err))
		} else {
			cfg.SecretAccessKey = decrypted
		}
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
