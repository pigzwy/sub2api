package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func firstOutputTestCache(t *testing.T) (*miniredis.Miniredis, *tempUnschedCache) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, &tempUnschedCache{rdb: client}
}

func TestFirstOutputCounterFastResetsWindowAndAccountsAreIsolated(t *testing.T) {
	server, cache := firstOutputTestCache(t)
	ctx := context.Background()
	count, claim, err := cache.RecordFirstOutput(ctx, 1, true, "a")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claim)
	server.FastForward(9 * time.Minute)
	count, _, err = cache.RecordFirstOutput(ctx, 1, true, "b")
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.Equal(t, time.Minute, server.TTL(firstOutputKeys(1)[0]))
	_, _, err = cache.RecordFirstOutput(ctx, 2, true, "other")
	require.NoError(t, err)
	_, _, err = cache.RecordFirstOutput(ctx, 1, false, "fast")
	require.NoError(t, err)
	require.False(t, server.Exists(firstOutputKeys(1)[0]))
	count, _, err = cache.RecordFirstOutput(ctx, 1, true, "new")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Equal(t, 10*time.Minute, server.TTL(firstOutputKeys(1)[0]))
	count, _, err = cache.RecordFirstOutput(ctx, 2, true, "other2")
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	server.FastForward(10 * time.Minute)
	count, claim, err = cache.RecordFirstOutput(ctx, 1, true, "expired")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claim)
}

func TestFirstOutputCounterConcurrentClaimsAndCooldown(t *testing.T) {
	server, cache := firstOutputTestCache(t)
	secondClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = secondClient.Close() })
	second := &tempUnschedCache{rdb: secondClient}
	ctx := context.Background()
	var claims atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := cache
			if i%2 == 0 {
				store = second
			}
			_, claimed, err := store.RecordFirstOutput(ctx, 1, true, "winner")
			if err != nil {
				t.Error(err)
			}
			if claimed {
				claims.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 1, claims.Load())
	require.NoError(t, cache.CompleteFirstOutputClaim(ctx, 1, "winner", time.Now().Add(5*time.Minute)))
	_, _, err := cache.RecordFirstOutput(ctx, 1, false, "late-fast")
	require.NoError(t, err)
	_, claimed, err := cache.RecordFirstOutput(ctx, 1, true, "late-slow")
	require.NoError(t, err)
	require.False(t, claimed)
	value, err := server.Get(firstOutputKeys(1)[1])
	require.NoError(t, err)
	require.Equal(t, "cooldown", value)
	server.FastForward(5 * time.Minute)
	count, claimed, err := cache.RecordFirstOutput(ctx, 1, true, "after-cooldown")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claimed)
}

func TestFirstOutputClaimFailureRetryAndManualRecovery(t *testing.T) {
	_, cache := firstOutputTestCache(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _, err := cache.RecordFirstOutput(ctx, 1, true, "failed")
		require.NoError(t, err)
	}
	acquired, err := cache.AcquireFirstOutputRecovery(ctx, 1, "manual")
	require.NoError(t, err)
	require.False(t, acquired, "manual recovery waits for the pending DB mutation")
	require.NoError(t, cache.ReleaseFirstOutputClaim(ctx, 1, "wrong-token"))
	_, claimed, err := cache.RecordFirstOutput(ctx, 1, true, "retry")
	require.NoError(t, err)
	require.False(t, claimed)
	require.NoError(t, cache.ReleaseFirstOutputClaim(ctx, 1, "failed"))
	count, claimed, err := cache.RecordFirstOutput(ctx, 1, true, "retry")
	require.NoError(t, err)
	require.EqualValues(t, 4, count)
	require.True(t, claimed)
	require.NoError(t, cache.CompleteFirstOutputClaim(ctx, 1, "retry", time.Now().Add(5*time.Minute)))
	acquired, err = cache.AcquireFirstOutputRecovery(ctx, 1, "manual")
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, cache.DeleteTempUnsched(ctx, 1))
	require.NoError(t, cache.ClearFirstOutputProtection(ctx, 1))
	count, claimed, err = cache.RecordFirstOutput(ctx, 1, true, "fresh")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claimed)
}

func TestFirstOutputConditionalPauseDoesNotOverwriteChangedAccount(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	applied, err := repo.SetTempUnschedulableIfUnchanged(context.Background(), 42, time.Now(), time.Now().Add(5*time.Minute), "first output")
	require.NoError(t, err)
	require.False(t, applied)
	require.Len(t, exec.execQueries, 1)
	require.Contains(t, exec.execQueries[0], "updated_at = $4")
	require.Contains(t, exec.execQueries[0], "temp_unschedulable_until <= NOW()")
}
