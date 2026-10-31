package repository

import (
	"context"
	"fmt"
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

var firstOutputRecoveryScript = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if value and value ~= 'cooldown' then return 0 end
redis.call('SET', KEYS[1], ARGV[1], 'EX', 15)
return 1
`)

func (c *tempUnschedCache) AcquireFirstOutputRecovery(ctx context.Context, accountID int64, token string) (bool, error) {
	value, err := firstOutputRecoveryScript.Run(ctx, c.rdb, []string{firstOutputKeys(accountID)[1]}, token).Int64()
	return value == 1, err
}

func firstOutputKeys(accountID int64) []string {
	base := fmt.Sprintf("temp_unsched:first_output:{%d}", accountID)
	return []string{base + ":count", base + ":claim"}
}

func (c *tempUnschedCache) RecordFirstOutput(ctx context.Context, accountID int64, slow bool, token string) (int64, bool, error) {
	signal := 0
	if slow {
		signal = 1
	}
	values, err := firstOutputRecordScript.Run(ctx, c.rdb, firstOutputKeys(accountID), signal, int(service.FirstOutputWindow.Seconds()), service.FirstOutputThreshold, token).Int64Slice()
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
	return firstOutputCompleteScript.Run(ctx, c.rdb, firstOutputKeys(accountID), token, ttl).Err()
}

func (c *tempUnschedCache) ReleaseFirstOutputClaim(ctx context.Context, accountID int64, token string) error {
	return firstOutputReleaseScript.Run(ctx, c.rdb, []string{firstOutputKeys(accountID)[1]}, token).Err()
}

func (c *tempUnschedCache) ClearFirstOutputProtection(ctx context.Context, accountID int64) error {
	return c.rdb.Del(ctx, firstOutputKeys(accountID)...).Err()
}
