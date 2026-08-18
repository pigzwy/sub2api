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

// mediaObjectStorageStub streams video and stores audio bytes, matching what
// production builds (*repository.S3ImageStorage implements both contracts).
type mediaObjectStorageStub struct {
	fakeAudioStorage
}

func (*mediaObjectStorageStub) UploadVideo(context.Context, string, string, io.Reader) error {
	return nil
}

func (*mediaObjectStorageStub) PresignVideo(context.Context, string) (string, time.Time, error) {
	return "https://storage.example.test/video", time.Now().Add(time.Hour), nil
}

func newMediaStorageFixture(t *testing.T, storage VideoObjectStorage) (*VideoStorageSettingService, *stubSettingRepo) {
	t.Helper()
	repo := newStubSettingRepo()
	encryptor := reversibleEncryptor{}
	backup := NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	}, encryptor, nil, nil)
	factory := func(context.Context, *config.VideoStorageConfig) (VideoObjectStorage, error) {
		return storage, nil
	}
	return NewVideoStorageSettingService(repo, encryptor, backup, factory, config.VideoStorageConfig{}), repo
}

func mediaStorageTarget() VideoStorageSettings {
	return VideoStorageSettings{
		Bucket: "media", Endpoint: "https://r2.example", Region: "auto",
		AccessKeyID: "ak", SecretAccessKey: "sk",
	}
}

func TestMediaStorageAudioSettingsRoundTrip(t *testing.T) {
	svc, _ := newMediaStorageFixture(t, &mediaObjectStorageStub{})
	in := mediaStorageTarget()
	in.Enabled, in.AudioEnabled, in.AudioPrefix = true, true, "/recordings"

	saved, err := svc.Update(t.Context(), in)
	require.NoError(t, err)
	require.True(t, saved.AudioEnabled)
	require.Equal(t, "recordings/", saved.AudioPrefix, "audio prefix is normalized like the video prefix")

	fetched, err := svc.Get(t.Context())
	require.NoError(t, err)
	require.True(t, fetched.AudioEnabled)
	require.Equal(t, "recordings/", fetched.AudioPrefix)

	storage, options, enabled := svc.ResolveAudio()
	require.True(t, enabled)
	require.NotNil(t, storage)
	require.Equal(t, "recordings/", options.Prefix)
}

func TestMediaStorageDefaultsAudioPrefixWhenBlank(t *testing.T) {
	svc, _ := newMediaStorageFixture(t, &mediaObjectStorageStub{})
	in := mediaStorageTarget()
	in.Enabled, in.AudioEnabled = true, true

	saved, err := svc.Update(t.Context(), in)
	require.NoError(t, err)
	require.Equal(t, "audio/", saved.AudioPrefix)
}

func TestMediaStorageLegacySettingsLeaveAudioDisabled(t *testing.T) {
	svc, repo := newMediaStorageFixture(t, &mediaObjectStorageStub{})
	// Exactly what the admin console persisted before audio existed: no
	// migration runs, so the missing fields must decode as "audio off".
	legacy := `{"enabled":true,"reuse_backup_s3":false,"bucket":"videos","prefix":"videos/",` +
		`"presign_expiry_hours":168,"max_download_bytes":536870912,"endpoint":"https://r2.example",` +
		`"region":"auto","access_key_id":"ak","secret_access_key":"enc:sk","force_path_style":true}`
	require.NoError(t, repo.Set(t.Context(), settingKeyVideoStorageConfig, legacy))

	_, videoOptions, videoEnabled := svc.resolve()
	require.True(t, videoEnabled, "video offload keeps working on pre-audio settings")
	require.Equal(t, "videos/", videoOptions.Prefix)

	_, _, audioEnabled := svc.ResolveAudio()
	require.False(t, audioEnabled)
}

func TestMediaStorageAudioRunsWithoutVideoOffload(t *testing.T) {
	svc, _ := newMediaStorageFixture(t, &mediaObjectStorageStub{})
	in := mediaStorageTarget()
	in.Enabled, in.AudioEnabled = false, true

	_, err := svc.Update(t.Context(), in)
	require.NoError(t, err)

	_, _, videoEnabled := svc.resolve()
	require.False(t, videoEnabled, "the video switch stays independent of audio")

	_, options, audioEnabled := svc.ResolveAudio()
	require.True(t, audioEnabled)
	require.Equal(t, "audio/", options.Prefix)
}

func TestMediaStorageResolveAudioFollowsInvalidate(t *testing.T) {
	svc, _ := newMediaStorageFixture(t, &mediaObjectStorageStub{})
	in := mediaStorageTarget()
	in.Enabled, in.AudioEnabled = true, true
	_, err := svc.Update(t.Context(), in)
	require.NoError(t, err)
	_, _, audioEnabled := svc.ResolveAudio()
	require.True(t, audioEnabled)

	in.AudioEnabled = false
	_, err = svc.Update(t.Context(), in)
	require.NoError(t, err)

	_, _, audioEnabled = svc.ResolveAudio()
	require.False(t, audioEnabled, "saving takes effect without a restart")
}

func TestMediaStorageResolveAudioRejectsClientThatCannotStoreBytes(t *testing.T) {
	// recordingVideoObjectStorage streams but has no Save; treating that as
	// enabled would drop every recording silently.
	svc, _ := newMediaStorageFixture(t, &recordingVideoObjectStorage{})
	in := mediaStorageTarget()
	in.Enabled, in.AudioEnabled = true, true
	_, err := svc.Update(t.Context(), in)
	require.NoError(t, err)

	_, _, videoEnabled := svc.resolve()
	require.True(t, videoEnabled)

	storage, _, audioEnabled := svc.ResolveAudio()
	require.False(t, audioEnabled)
	require.Nil(t, storage)
}
