package repository

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var firstOutputRecordScript = redis.NewScript(`
if ARGV[1] == '0' then
  redis.call('DEL', KEYS[1])
  return {0, 0}
end
if redis.call('EXISTS', KEYS[2]) == 1 then return {0, 0} end
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('EXPIRE', KEYS[1], ARGV[2]) end
if count < tonumber(ARGV[3]) then return {count, 0} end
redis.call('SET', KEYS[2], ARGV[4], 'EX', 15)
return {count, 1}
`)

var firstOutputCompleteScript = redis.NewScript(`
if redis.call('GET', KEYS[2]) ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], 'cooldown', 'PX', ARGV[2])
return 1
`)

var firstOutputReleaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then redis.call('DEL', KEYS[1]) end
return 1
`)

func firstOutputKeys(accountID int64) []string {
	base := fmt.Sprintf("temp_unsched:first_output:{%d}", accountID)
	return []string{base + ":count", base + ":claim"}
}

// Each manual recovery starts a fresh namespace; old keys expire naturally.
func firstOutputTokenKeys(accountID int64, token string) []string {
	keys := firstOutputKeys(accountID)
	if generation, _, ok := strings.Cut(token, "/"); ok && generation != "" {
		suffix := fmt.Sprintf(":%x", sha256.Sum256([]byte(generation)))
		keys[0] += suffix
		keys[1] += suffix
	}
	return keys
}

func (c *tempUnschedCache) RecordFirstOutput(ctx context.Context, accountID int64, slow bool, token string, window time.Duration) (int64, bool, error) {
	signal := 0
	if slow {
		signal = 1
	}
	values, err := firstOutputRecordScript.Run(ctx, c.rdb, firstOutputTokenKeys(accountID, token), signal, int(window.Seconds()), service.FirstOutputThreshold, token).Int64Slice()
	if err != nil {
		return 0, false, err
	}
	if len(values) != 2 {
		return 0, false, fmt.Errorf("unexpected first output counter result")
	}
	return values[0], values[1] == 1, nil
}

func (c *tempUnschedCache) CompleteFirstOutputClaim(ctx context.Context, accountID int64, token string, until time.Time) error {
	ttl := max(time.Until(until).Milliseconds(), 1)
	return firstOutputCompleteScript.Run(ctx, c.rdb, firstOutputTokenKeys(accountID, token), token, ttl).Err()
}

func (c *tempUnschedCache) ReleaseFirstOutputClaim(ctx context.Context, accountID int64, token string) error {
	return firstOutputReleaseScript.Run(ctx, c.rdb, []string{firstOutputTokenKeys(accountID, token)[1]}, token).Err()
}

func (c *tempUnschedCache) ClearFirstOutputProtection(ctx context.Context, accountID int64) error {
	return c.rdb.Del(ctx, firstOutputKeys(accountID)...).Err()
}
