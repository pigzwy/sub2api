//go:build unit

package service

import (
	"context"
	"encoding/json"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFirstOutputConfigDefaultsValidationAndPartialPatch(t *testing.T) {
	defaults := DefaultFirstOutputProtectionConfig()
	require.Equal(t, FirstOutputProtectionConfig{30, 5, 10}, (*Account)(nil).FirstOutputProtectionConfig())
	require.NoError(t, NormalizeFirstOutputProtectionCredentials(nil))
	for _, invalid := range []any{nil, "45", true, 0, -1, 1.5, math.NaN(), math.Inf(1), 3601} {
		credentials := map[string]any{FirstOutputLimitSecondsKey: invalid}
		require.Error(t, NormalizeFirstOutputProtectionCredentials(credentials), "%v", invalid)
		require.Equal(t, defaults, (&Account{Credentials: credentials}).FirstOutputProtectionConfig())
	}
	credentials := map[string]any{FirstOutputLimitSecondsKey: json.Number("45"), FirstOutputCooldownMinutesKey: float64(12), FirstOutputWindowMinutesKey: int64(20), "api_key": "keep"}
	require.NoError(t, NormalizeFirstOutputProtectionCredentials(credentials))
	require.Equal(t, FirstOutputProtectionConfig{45, 12, 20}, (&Account{Credentials: credentials}).FirstOutputProtectionConfig())
	patch := map[string]any{FirstOutputCooldownMinutesKey: 7}
	require.NoError(t, NormalizeFirstOutputProtectionCredentials(patch))
	require.Len(t, patch, 1, "bulk validation must not fill omitted fields")
	for _, key := range []string{FirstOutputCooldownMinutesKey, FirstOutputWindowMinutesKey} {
		require.Error(t, NormalizeFirstOutputProtectionCredentials(map[string]any{key: 1441}))
		require.NoError(t, NormalizeFirstOutputProtectionCredentials(map[string]any{key: 1440}))
	}
}

func TestFirstOutputConfigCustomThresholdWindowAndCooldown(t *testing.T) {
	svc, cache, repo := firstOutputFixture()
	repo.account.Credentials[FirstOutputLimitSecondsKey] = 45
	repo.account.Credentials[FirstOutputWindowMinutesKey] = 20
	repo.account.Credentials[FirstOutputCooldownMinutesKey] = 12
	for _, seconds := range []int{46, 47, 44, 46, 45, 48} {
		svc.RecordFirstOutputLatency(context.Background(), repo.account, "model", time.Duration(seconds)*time.Second)
	}
	require.Zero(t, repo.writes)
	require.EqualValues(t, 2, cache.count)
	require.Equal(t, []bool{true, true, false, true, true}, cache.signals)
	for _, window := range cache.windows {
		require.Equal(t, 20*time.Minute, window)
	}
	svc.RecordFirstOutputLatency(context.Background(), repo.account, "model", 46*time.Second)
	require.Equal(t, 1, repo.writes)
	require.InDelta(t, 12*time.Minute, time.Until(*repo.account.TempUnschedulableUntil), float64(time.Second))
	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(repo.account.TempUnschedulableReason), &state))
	require.Equal(t, 45, state.FirstOutputLimitSeconds)
	require.Equal(t, 12, state.CooldownMinutes)
	require.Equal(t, 20, state.TriggerWindowMinutes)
	require.Contains(t, state.ErrorMessage, "45 seconds")
	require.Contains(t, state.ErrorMessage, "20 minutes")
}

func TestFirstOutputObserverKeepsConfigSnapshotAndRejectsObsoleteTrip(t *testing.T) {
	svc, cache, repo := firstOutputFixture()
	repo.account.Credentials[FirstOutputLimitSecondsKey] = 45
	repo.account.Credentials[FirstOutputWindowMinutesKey] = 20
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	finish := beginFirstOutputProtection(context.Background(), c, repo.account, "model", true, svc)
	defer finish()
	StartFirstOutputProtection(firstOutputProtectionContext(context.Background(), c))
	value, _ := c.Get(firstOutputObserverKey)
	observer, ok := value.(*firstOutputObservation)
	require.True(t, ok)
	require.Equal(t, 45, observer.config.LimitSeconds)
	repo.account.Credentials[FirstOutputLimitSecondsKey] = 60
	observer.mu.Lock()
	observer.start = time.Now().Add(-50 * time.Second)
	observer.mu.Unlock()
	observeFirstOutput(c)
	require.Equal(t, []bool{true}, cache.signals, "50 seconds is slow under the captured 45-second policy")
	require.Equal(t, 20*time.Minute, cache.windows[0])
	// A third late sample under the old policy must not pause the new one.
	oldConfig := observer.config
	svc.recordFirstOutputLatency(context.Background(), repo.account, "model", 50*time.Second, oldConfig)
	svc.recordFirstOutputLatency(context.Background(), repo.account, "model", 50*time.Second, oldConfig)
	require.Zero(t, repo.writes)
	require.Empty(t, cache.claim)
}

func TestFirstOutputObserverUsesConfiguredTimer(t *testing.T) {
	svc, cache, repo := firstOutputFixture()
	repo.account.Credentials[FirstOutputLimitSecondsKey] = 1
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	finish := beginFirstOutputProtection(context.Background(), c, repo.account, "model", true, svc)
	defer finish()
	StartFirstOutputProtection(firstOutputProtectionContext(context.Background(), c))
	require.Eventually(t, func() bool { cache.mu.Lock(); defer cache.mu.Unlock(); return len(cache.signals) == 1 }, 2*time.Second, 5*time.Millisecond)
	finish()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, []bool{true}, cache.signals)
}
