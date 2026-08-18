//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
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

// probingVideoObjectStorage lets TestConnection tests dictate the probe result.
type probingVideoObjectStorage struct {
	recordingVideoObjectStorage
	prefixes []string
	err      error
}

func (p *probingVideoObjectStorage) ProbeVideoStorage(_ context.Context, prefix string) error {
	p.prefixes = append(p.prefixes, prefix)
	return p.err
}

func TestVideoStorageResolveFailureIsRetriedWithoutRestart(t *testing.T) {
	repo := newStubSettingRepo()
	encryptor := reversibleEncryptor{}
	backup := NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	}, encryptor, nil, nil)
	fail := true
	calls := 0
	factory := func(_ context.Context, _ *config.VideoStorageConfig) (VideoObjectStorage, error) {
		calls++
		if fail {
			return nil, errors.New("endpoint temporarily down")
		}
		return &recordingVideoObjectStorage{}, nil
	}
	svc := NewVideoStorageSettingService(repo, encryptor, backup, factory, config.VideoStorageConfig{
		Enabled: true, Endpoint: "https://video.r2.example", Region: "auto",
		Bucket: "yaml-video", AccessKeyID: "ak", SecretAccessKey: "sk", Prefix: "videos/",
	})

	_, _, enabled := svc.resolve()
	require.False(t, enabled)

	// The endpoint comes back; no Invalidate, no restart.
	fail = false
	_, _, enabled = svc.resolve()
	require.True(t, enabled, "a transient factory failure must not pin offload to disabled")

	// Success is cached: another resolve must not rebuild the client.
	_, _, enabled = svc.resolve()
	require.True(t, enabled)
	require.Equal(t, 2, calls)
}

func TestVideoStorageResolveFailureLogsAreThrottled(t *testing.T) {
	svc, _, _ := newVideoStorageFixture(t, config.VideoStorageConfig{})
	at := time.Unix(1700000000, 0)
	svc.now = func() time.Time { return at }

	require.True(t, svc.shouldLogFailure("client_build"), "first failure must log")
	require.False(t, svc.shouldLogFailure("client_build"), "repeat within the interval is throttled")
	at = at.Add(videoStorageFailureLogInterval + time.Second)
	require.True(t, svc.shouldLogFailure("client_build"), "logging resumes once the interval passes")
	require.True(t, svc.shouldLogFailure("settings_load"), "a different failure kind logs immediately")
}

func TestVideoStorageStoredSecretDecryptFailureFailsClosed(t *testing.T) {
	svc, repo, built := newVideoStorageFixture(t, config.VideoStorageConfig{})
	ctx := context.Background()
	_, err := svc.Update(ctx, VideoStorageSettings{
		Enabled: true, Bucket: "videos", Endpoint: "https://video.r2.example",
		AccessKeyID: "ak", SecretAccessKey: "video-secret",
	})
	require.NoError(t, err)

	// Simulate a rotated or wrong encryption key: the ciphertext no longer decrypts.
	raw, err := repo.GetValue(ctx, settingKeyVideoStorageConfig)
	require.NoError(t, err)
	require.NoError(t, repo.Set(ctx, settingKeyVideoStorageConfig,
		strings.ReplaceAll(raw, "enc:video-secret", "corrupted")))
	svc.Invalidate()

	_, _, enabled := svc.resolve()
	require.False(t, enabled, "an unreadable secret must disable offload, not sign with ciphertext")
	require.Empty(t, *built, "the factory must never see the undecryptable value")

	err = svc.TestConnection(ctx, VideoStorageSettings{
		Enabled: true, Bucket: "videos", Endpoint: "https://video.r2.example", AccessKeyID: "ak",
	})
	require.ErrorIs(t, err, ErrVideoStorageSecretUnreadable)
}

func TestVideoStorageTestConnectionRunsARealProbe(t *testing.T) {
	repo := newStubSettingRepo()
	encryptor := reversibleEncryptor{}
	backup := NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	}, encryptor, nil, nil)
	probe := &probingVideoObjectStorage{}
	var built []config.VideoStorageConfig
	factory := func(_ context.Context, cfg *config.VideoStorageConfig) (VideoObjectStorage, error) {
		built = append(built, *cfg)
		return probe, nil
	}
	svc := NewVideoStorageSettingService(repo, encryptor, backup, factory, config.VideoStorageConfig{})
	ctx := context.Background()

	in := VideoStorageSettings{
		Enabled: true, Bucket: "videos", Endpoint: "https://video.r2.example",
		AccessKeyID: "ak", SecretAccessKey: "typed-secret",
	}
	require.NoError(t, svc.TestConnection(ctx, in))
	require.Equal(t, []string{"videos/"}, probe.prefixes, "the probe must write under the normalized prefix")
	// The typed secret is plaintext from the admin: it must reach the client
	// verbatim instead of failing a decrypt it never went through.
	require.Equal(t, "typed-secret", built[0].SecretAccessKey)

	probe.err = fmt.Errorf("%w: boom", ErrVideoStorageAccessDenied)
	require.ErrorIs(t, svc.TestConnection(ctx, in), ErrVideoStorageAccessDenied)
}

func TestVideoStorageTestConnectionRefusesUnprobeableStorage(t *testing.T) {
	svc, _, _ := newVideoStorageFixture(t, config.VideoStorageConfig{})
	err := svc.TestConnection(context.Background(), VideoStorageSettings{
		Enabled: true, Bucket: "videos", Endpoint: "https://video.r2.example",
		AccessKeyID: "ak", SecretAccessKey: "sk",
	})
	require.ErrorIs(t, err, ErrVideoStorageNotProbeable)
}
