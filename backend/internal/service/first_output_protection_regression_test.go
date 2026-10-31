//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFirstOutputPreparationDoesNotStartClock(t *testing.T) {
	s, cache, repo := firstOutputFixture()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	finish := beginFirstOutputProtection(context.Background(), c, repo.account, "model", true, s)
	defer finish()
	value, _ := c.Get(firstOutputObserverKey)
	observer := value.(*firstOutputObservation)
	require.True(t, observer.start.IsZero())
	require.Nil(t, observer.timer)
	StartFirstOutputProtection(firstOutputProtectionContext(context.Background(), c))
	start := observer.start
	require.False(t, start.IsZero())
	StartFirstOutputProtection(firstOutputProtectionContext(context.Background(), c))
	require.Equal(t, start, observer.start, "nested transports must not restart the deadline")
	observeFirstOutputPayload(c, []byte(`{"choices":[{"delta":{"refusal":"Cannot comply"}}]}`))
	require.Equal(t, []bool{false}, cache.signals)
}

func TestFirstOutputNativeAndPassthroughIgnoreEmptyItemsEvenWithSemanticTTFT(t *testing.T) {
	previous := gatewayForwardingCache.Load()
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{openAITTFTMode: OpenAITTFTModeSemantic, expiresAt: time.Now().Add(time.Minute).UnixNano()})
	t.Cleanup(func() {
		if previous != nil {
			gatewayForwardingCache.Store(previous)
		} else {
			gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
		}
	})
	for _, passthrough := range []bool{false, true} {
		for _, refusal := range []bool{false, true} {
			s, cache, repo := firstOutputFixture()
			repo.account.Platform = PlatformOpenAI
			cache.count = 2
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			finish := beginFirstOutputProtection(c.Request.Context(), c, repo.account, "model", true, s)
			ctx := firstOutputProtectionContext(c.Request.Context(), c)
			StartFirstOutputProtection(ctx)
			payload := "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\",\"content\":[]}}\n\n"
			if refusal {
				payload += "data: {\"type\":\"response.refusal.delta\",\"delta\":\"Cannot comply\"}\n\n"
			}
			payload += "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}
			cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
			gateway := &OpenAIGatewayService{cfg: cfg, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
			var err error
			if passthrough {
				_, err = gateway.handleStreamingResponsePassthrough(ctx, resp, c, repo.account, time.Now(), "model", "model")
			} else {
				_, err = gateway.handleStreamingResponse(ctx, resp, c, repo.account, time.Now(), "model", "model")
			}
			require.NoError(t, err)
			finish()
			if refusal {
				require.Zero(t, cache.count)
			} else {
				require.EqualValues(t, 2, cache.count)
			}
		}
	}
}

func TestFirstOutputAutomaticRecoveryPreservesCooldownAndCount(t *testing.T) {
	until := time.Now().Add(FirstOutputCooldown)
	for _, options := range []AccountRecoveryOptions{{PreserveFirstOutputCooldown: true}, {InvalidateToken: true, PreserveFirstOutputCooldown: true}} {
		repo := &rateLimitClearRepoStub{getByIDAccount: &Account{ID: 42, Status: StatusActive, TempUnschedulableUntil: &until, TempUnschedulableReason: `{"matched_keyword":"first_output_timeout"}`}}
		cache := &firstOutputCacheStub{count: 2}
		svc := NewRateLimitService(repo, nil, nil, nil, cache)
		result, err := svc.RecoverAccountState(context.Background(), 42, options)
		require.NoError(t, err)
		require.False(t, result.ClearedRateLimit)
		require.Zero(t, repo.clearTempUnschedCalls)
		require.EqualValues(t, 2, cache.count)
	}
}

type unavailableFirstOutputCache struct{ firstOutputCacheStub }

func (*unavailableFirstOutputCache) AcquireFirstOutputRecovery(context.Context, int64, string) (bool, error) {
	return false, errors.New("redis unavailable")
}
func (*unavailableFirstOutputCache) ClearFirstOutputProtection(context.Context, int64) error {
	return errors.New("redis unavailable")
}
func (*unavailableFirstOutputCache) DeleteTempUnsched(context.Context, int64) error {
	return errors.New("redis unavailable")
}

func TestFirstOutputRedisOutageDoesNotBlockManualRecovery(t *testing.T) {
	repo := &rateLimitClearRepoStub{}
	svc := NewRateLimitService(repo, nil, nil, nil, &unavailableFirstOutputCache{})
	require.NoError(t, svc.ClearTempUnschedulable(context.Background(), 42))
	require.Equal(t, 1, repo.clearTempUnschedCalls)
}

func TestFirstOutputOldRequestCannotUndoManualRecovery(t *testing.T) {
	svc, cache, repo := firstOutputFixture()
	old := *repo.account
	repo.account.Extra = map[string]any{"first_output_recovery_generation": "new-generation"}
	for i := 0; i < 3; i++ {
		svc.RecordFirstOutputLatency(context.Background(), &old, "model", 31*time.Second)
	}
	require.Zero(t, repo.writes)
	require.Empty(t, cache.claim)
}

func TestFirstOutputIgnoresEmptyToolMetadataAndImageParts(t *testing.T) {
	for _, payload := range []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0}]}}]}`,
		`{"choices":[{"delta":{"function_call":{}}}]}`,
		`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"x"}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{}}]}}]}`,
	} {
		require.False(t, firstOutputPayloadIsMeaningful([]byte(payload)), payload)
	}
}
