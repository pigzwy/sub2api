//go:build unit

package service

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type recordingVideoObjectStorage struct{}

func (*recordingVideoObjectStorage) UploadVideo(context.Context, string, string, io.Reader) error {
	return nil
}

func (*recordingVideoObjectStorage) PresignVideo(context.Context, string) (string, time.Time, error) {
	return "https://storage.example.test/video", time.Now().Add(time.Hour), nil
}

func newVideoStorageFixture(t *testing.T, fallback config.VideoStorageConfig) (*VideoStorageSettingService, *stubSettingRepo, *[]config.VideoStorageConfig) {
	svc, repo, built, _ := newVideoStorageFixtureWithBackup(t, fallback)
	return svc, repo, built
}

func newVideoStorageFixtureWithBackup(t *testing.T, fallback config.VideoStorageConfig) (*VideoStorageSettingService, *stubSettingRepo, *[]config.VideoStorageConfig, *BackupService) {
	t.Helper()
	repo := newStubSettingRepo()
	encryptor := reversibleEncryptor{}
	backup := NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	}, encryptor, nil, nil)
	var built []config.VideoStorageConfig
	factory := func(_ context.Context, cfg *config.VideoStorageConfig) (VideoObjectStorage, error) {
		built = append(built, *cfg)
		return &recordingVideoObjectStorage{}, nil
	}
	return NewVideoStorageSettingService(repo, encryptor, backup, factory, fallback), repo, &built, backup
}

func TestVideoStorageSettingsIndependentToggleAndTarget(t *testing.T) {
	svc, repo, built := newVideoStorageFixture(t, config.VideoStorageConfig{})
	ctx := context.Background()
	seedBackupS3(t, repo, BackupS3Config{
		Endpoint: "https://backup.r2.example", Region: "auto", Bucket: "backup-bucket",
		AccessKeyID: "backup-ak", SecretAccessKey: "backup-sk",
	})

	storage, _, enabled := svc.resolve()
	require.False(t, enabled)
	require.Nil(t, storage)

	_, err := svc.Update(ctx, VideoStorageSettings{
		Enabled: true, ReuseBackupS3: true, Bucket: "video-bucket",
		Prefix: "completed/", PresignExpiry: 168, MaxDownloadBytes: 700 << 20,
	})
	require.NoError(t, err)
	storage, options, enabled := svc.resolve()
	require.True(t, enabled)
	require.NotNil(t, storage)
	require.Equal(t, "completed/", options.Prefix)
	require.Equal(t, int64(700<<20), options.MaxDownloadBytes)
	require.Len(t, *built, 1)
	require.Equal(t, "video-bucket", (*built)[0].Bucket)
	require.Equal(t, "backup-ak", (*built)[0].AccessKeyID)
	require.Equal(t, "backup-sk", (*built)[0].SecretAccessKey)
	require.Equal(t, 168, (*built)[0].PresignExpiry)

	raw, err := repo.GetValue(ctx, settingKeyVideoStorageConfig)
	require.NoError(t, err)
	require.NotContains(t, raw, "backup-sk")

	_, err = svc.Update(ctx, VideoStorageSettings{Enabled: false})
	require.NoError(t, err)
	_, _, enabled = svc.resolve()
	require.False(t, enabled)
}

func TestVideoStorageSettingsOwnCredentialsAreEncryptedAndMasked(t *testing.T) {
	svc, repo, built := newVideoStorageFixture(t, config.VideoStorageConfig{})
	ctx := context.Background()
	saved, err := svc.Update(ctx, VideoStorageSettings{
		Enabled: true, Bucket: "videos", Endpoint: "https://video.r2.example",
		AccessKeyID: "video-ak", SecretAccessKey: "video-secret",
	})
	require.NoError(t, err)
	require.Empty(t, saved.SecretAccessKey)
	raw, err := repo.GetValue(ctx, settingKeyVideoStorageConfig)
	require.NoError(t, err)
	require.Contains(t, raw, "enc:video-secret")
	require.NotContains(t, raw, `"secret_access_key":"video-secret"`)
	fetched, err := svc.Get(ctx)
	require.NoError(t, err)
	require.Empty(t, fetched.SecretAccessKey)
	require.True(t, svc.SecretConfigured(ctx))
	_, _, enabled := svc.resolve()
	require.True(t, enabled)
	require.Equal(t, "video-secret", (*built)[0].SecretAccessKey)
}

func TestVideoStorageSettingsFallBackToIndependentConfig(t *testing.T) {
	svc, _, built := newVideoStorageFixture(t, config.VideoStorageConfig{
		Enabled: true, Endpoint: "https://video.r2.example", Region: "auto",
		Bucket: "yaml-video", AccessKeyID: "ak", SecretAccessKey: "sk",
		Prefix: "videos/", MaxDownloadByte: defaultVideoMaxDownloadBytes,
	})
	_, options, enabled := svc.resolve()
	require.True(t, enabled)
	require.Equal(t, "videos/", (*built)[0].Prefix)
	require.Equal(t, "videos/", options.Prefix)
}

func TestVideoStorageSettingsSavePreservesFallbackSecret(t *testing.T) {
	svc, repo, built := newVideoStorageFixture(t, config.VideoStorageConfig{
		Enabled: true, Endpoint: "https://video.r2.example", Region: "auto",
		Bucket: "yaml-video", AccessKeyID: "yaml-ak", SecretAccessKey: "yaml-secret",
		Prefix: "videos/", MaxDownloadByte: defaultVideoMaxDownloadBytes,
	})

	_, err := svc.Update(t.Context(), VideoStorageSettings{
		Enabled: true, Bucket: "edited-video", Endpoint: "https://video.r2.example",
		Region: "auto", AccessKeyID: "yaml-ak", Prefix: "completed/",
	})
	require.NoError(t, err)
	raw, err := repo.GetValue(t.Context(), settingKeyVideoStorageConfig)
	require.NoError(t, err)
	require.Contains(t, raw, "enc:yaml-secret")
	_, _, enabled := svc.resolve()
	require.True(t, enabled)
	require.Equal(t, "yaml-secret", (*built)[0].SecretAccessKey)
}

func TestVideoStorageReuseBackupRebuildsAfterBackupCredentialUpdate(t *testing.T) {
	svc, repo, built, backup := newVideoStorageFixtureWithBackup(t, config.VideoStorageConfig{})
	seedBackupS3(t, repo, BackupS3Config{
		Endpoint: "https://old.r2.example", Region: "auto", Bucket: "backup",
		AccessKeyID: "old-ak", SecretAccessKey: "old-sk",
	})
	_, err := svc.Update(t.Context(), VideoStorageSettings{Enabled: true, ReuseBackupS3: true})
	require.NoError(t, err)
	_, _, enabled := svc.resolve()
	require.True(t, enabled)
	require.Equal(t, "old-ak", (*built)[0].AccessKeyID)

	_, err = backup.UpdateS3Config(t.Context(), BackupS3Config{
		Endpoint: "https://new.r2.example", Region: "auto", Bucket: "backup",
		AccessKeyID: "new-ak", SecretAccessKey: "new-sk",
	})
	require.NoError(t, err)
	_, _, enabled = svc.resolve()
	require.True(t, enabled)
	require.Len(t, *built, 2, "backup credential updates must rebuild the video client immediately")
	require.Equal(t, "new-ak", (*built)[1].AccessKeyID)
	require.Equal(t, "new-sk", (*built)[1].SecretAccessKey)
}
