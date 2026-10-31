package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
	count, claim, err := cache.RecordFirstOutput(ctx, 1, true, "a", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claim)
	server.FastForward(9 * time.Minute)
	count, _, err = cache.RecordFirstOutput(ctx, 1, true, "b", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.Equal(t, time.Minute, server.TTL(firstOutputKeys(1)[0]))
	_, _, err = cache.RecordFirstOutput(ctx, 2, true, "other", service.FirstOutputWindow)
	require.NoError(t, err)
	_, _, err = cache.RecordFirstOutput(ctx, 1, false, "fast", service.FirstOutputWindow)
	require.NoError(t, err)
	require.False(t, server.Exists(firstOutputKeys(1)[0]))
	count, _, err = cache.RecordFirstOutput(ctx, 1, true, "new", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Equal(t, 10*time.Minute, server.TTL(firstOutputKeys(1)[0]))
	count, _, err = cache.RecordFirstOutput(ctx, 2, true, "other2", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	server.FastForward(10 * time.Minute)
	count, claim, err = cache.RecordFirstOutput(ctx, 1, true, "expired", service.FirstOutputWindow)
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
			_, claimed, err := store.RecordFirstOutput(ctx, 1, true, "winner", service.FirstOutputWindow)
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
	_, _, err := cache.RecordFirstOutput(ctx, 1, false, "late-fast", service.FirstOutputWindow)
	require.NoError(t, err)
	_, claimed, err := cache.RecordFirstOutput(ctx, 1, true, "late-slow", service.FirstOutputWindow)
	require.NoError(t, err)
	require.False(t, claimed)
	value, err := server.Get(firstOutputKeys(1)[1])
	require.NoError(t, err)
	require.Equal(t, "cooldown", value)
	server.FastForward(5 * time.Minute)
	count, claimed, err := cache.RecordFirstOutput(ctx, 1, true, "after-cooldown", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claimed)
}

func TestFirstOutputClaimFailureRetryAndManualRecovery(t *testing.T) {
	_, cache := firstOutputTestCache(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _, err := cache.RecordFirstOutput(ctx, 1, true, "failed", service.FirstOutputWindow)
		require.NoError(t, err)
	}
	require.NoError(t, cache.ReleaseFirstOutputClaim(ctx, 1, "wrong-token"))
	_, claimed, err := cache.RecordFirstOutput(ctx, 1, true, "retry", service.FirstOutputWindow)
	require.NoError(t, err)
	require.False(t, claimed)
	require.NoError(t, cache.ReleaseFirstOutputClaim(ctx, 1, "failed"))
	count, claimed, err := cache.RecordFirstOutput(ctx, 1, true, "retry", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 4, count)
	require.True(t, claimed)
	require.NoError(t, cache.CompleteFirstOutputClaim(ctx, 1, "retry", time.Now().Add(5*time.Minute)))
	require.NoError(t, cache.DeleteTempUnsched(ctx, 1))
	require.NoError(t, cache.ClearFirstOutputProtection(ctx, 1))
	count, claimed, err = cache.RecordFirstOutput(ctx, 1, true, "fresh", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claimed)
}

func TestFirstOutputConditionalPauseDoesNotOverwriteChangedAccount(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	applied, err := repo.SetTempUnschedulableIfUnchanged(context.Background(), 42, "generation", service.DefaultFirstOutputProtectionConfig(), time.Now().Add(5*time.Minute), "first output")
	require.NoError(t, err)
	require.False(t, applied)
	require.Len(t, exec.execQueries, 1)
	require.Contains(t, exec.execQueries[0], "first_output_recovery_generation")
	require.Contains(t, exec.execQueries[0], "temp_unschedulable_until <= NOW()")
}

func TestFirstOutputRecoveryGenerationIsolatesOldCountsAndClaims(t *testing.T) {
	_, cache := firstOutputTestCache(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _, err := cache.RecordFirstOutput(ctx, 42, true, "old/claim", service.FirstOutputWindow)
		require.NoError(t, err)
	}
	count, claimed, err := cache.RecordFirstOutput(ctx, 42, true, "new/claim", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claimed)
	require.NoError(t, cache.CompleteFirstOutputClaim(ctx, 42, "old/claim", time.Now().Add(5*time.Minute)))
	_, _, err = cache.RecordFirstOutput(ctx, 42, false, "old/fast", service.FirstOutputWindow)
	require.NoError(t, err)
	count, claimed, err = cache.RecordFirstOutput(ctx, 42, true, "new/claim2", service.FirstOutputWindow)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.False(t, claimed)
}

func TestFirstOutputCustomWindowAndPolicyNamespaces(t *testing.T) {
	server, cache := firstOutputTestCache(t)
	ctx := t.Context()
	oldToken := "generation|45:12:20/old"
	for i := 0; i < 2; i++ {
		_, _, err := cache.RecordFirstOutput(ctx, 42, true, oldToken, 20*time.Minute)
		require.NoError(t, err)
	}
	server.FastForward(19 * time.Minute)
	require.Equal(t, time.Minute, server.TTL(firstOutputTokenKeys(42, oldToken)[0]))
	count, claimed, err := cache.RecordFirstOutput(ctx, 42, true, "generation|60:7:4/new", 4*time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, claimed)
	require.Equal(t, 4*time.Minute, server.TTL(firstOutputTokenKeys(42, "generation|60:7:4/new")[0]))
	_, _, err = cache.RecordFirstOutput(ctx, 42, false, oldToken, 20*time.Minute)
	require.NoError(t, err)
	count, _, err = cache.RecordFirstOutput(ctx, 42, true, oldToken, 20*time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Equal(t, 20*time.Minute, server.TTL(firstOutputTokenKeys(42, oldToken)[0]))
	server.FastForward(4 * time.Minute)
	count, _, err = cache.RecordFirstOutput(ctx, 42, true, "generation|60:7:4/new", 4*time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}
