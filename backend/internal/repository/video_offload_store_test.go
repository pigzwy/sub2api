//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestVideoOffloadStoreLockRecordAndTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewVideoOffloadStore(rdb)
	ctx := context.Background()

	won, err := store.TryLock(ctx, "request/one", 10*time.Minute)
	require.NoError(t, err)
	require.True(t, won)
	won, err = store.TryLock(ctx, "request/one", 10*time.Minute)
	require.NoError(t, err)
	require.False(t, won)

	record := &service.VideoOffloadRecord{S3Key: "images/videos/request%2Fone.mp4", UploadedAt: 1700000000000}
	require.NoError(t, store.Save(ctx, "request/one", record, 24*time.Hour))
	got, err := store.Get(ctx, "request/one")
	require.NoError(t, err)
	require.Equal(t, record, got)

	key, err := videoOffloadRedisKey(videoOffloadRecordKeyPrefix, "request/one")
	require.NoError(t, err)
	require.Equal(t, 24*time.Hour, mr.TTL(key))
}
