//go:build unit

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadVideoStorageFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("VIDEO_STORAGE_ENABLED", "true")
	t.Setenv("VIDEO_STORAGE_ENDPOINT", "https://video.r2.cloudflarestorage.com")
	t.Setenv("VIDEO_STORAGE_BUCKET", "my-videos")
	t.Setenv("VIDEO_STORAGE_ACCESS_KEY_ID", "video-ak")
	t.Setenv("VIDEO_STORAGE_SECRET_ACCESS_KEY", "video-sk")
	t.Setenv("VIDEO_STORAGE_PREFIX", "completed/")
	t.Setenv("VIDEO_STORAGE_MAX_DOWNLOAD_BYTES", "734003200")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.VideoStorage.Enabled)
	require.Equal(t, "https://video.r2.cloudflarestorage.com", cfg.VideoStorage.Endpoint)
	require.Equal(t, "my-videos", cfg.VideoStorage.Bucket)
	require.Equal(t, "video-ak", cfg.VideoStorage.AccessKeyID)
	require.Equal(t, "video-sk", cfg.VideoStorage.SecretAccessKey)
	require.Equal(t, "completed/", cfg.VideoStorage.Prefix)
	require.Equal(t, int64(734003200), cfg.VideoStorage.MaxDownloadByte)
	require.True(t, cfg.VideoStorage.Active())
}
