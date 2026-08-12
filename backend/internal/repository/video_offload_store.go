package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	videoOffloadLockKeyPrefix = "grok_video_offload:lock:"
	// v2 records belong to the independent video_storage target. Legacy records
	// were created against image_storage and must never be presigned through a
	// different bucket/client after upgrade.
	videoOffloadRecordKeyPrefix = "grok_video_offload:record:v2:"
)

type videoOffloadStore struct {
	rdb *redis.Client
}

func NewVideoOffloadStore(rdb *redis.Client) service.VideoOffloadStore {
	return &videoOffloadStore{rdb: rdb}
}

func (s *videoOffloadStore) TryLock(ctx context.Context, requestID string, ttl time.Duration) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("video offload store unavailable")
	}
	key, err := videoOffloadRedisKey(videoOffloadLockKeyPrefix, requestID)
	if err != nil {
		return false, err
	}
	return s.rdb.SetNX(ctx, key, "1", ttl).Result()
}

func (s *videoOffloadStore) Get(ctx context.Context, requestID string) (*service.VideoOffloadRecord, error) {
	if s == nil || s.rdb == nil {
		return nil, errors.New("video offload store unavailable")
	}
	key, err := videoOffloadRedisKey(videoOffloadRecordKeyPrefix, requestID)
	if err != nil {
		return nil, err
	}
	data, err := s.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var record service.VideoOffloadRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *videoOffloadStore) Save(ctx context.Context, requestID string, record *service.VideoOffloadRecord, ttl time.Duration) error {
	if s == nil || s.rdb == nil {
		return errors.New("video offload store unavailable")
	}
	if record == nil || strings.TrimSpace(record.S3Key) == "" {
		return errors.New("video offload record is incomplete")
	}
	key, err := videoOffloadRedisKey(videoOffloadRecordKeyPrefix, requestID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, key, data, ttl).Err()
}

func videoOffloadRedisKey(prefix, requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", errors.New("video offload request ID is empty")
	}
	sum := sha256.Sum256([]byte(requestID))
	return prefix + hex.EncodeToString(sum[:]), nil
}
