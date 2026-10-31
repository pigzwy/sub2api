//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type firstOutputCacheStub struct {
	TempUnschedCache
	mu        sync.Mutex
	count     int64
	claim     string
	signals   []bool
	completed int
	state     *TempUnschedState
}

func (c *firstOutputCacheStub) RecordFirstOutput(_ context.Context, _ int64, slow bool, token string) (int64, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.signals = append(c.signals, slow)
	if !slow {
		c.count = 0
		return 0, false, nil
	}
	if c.claim != "" {
		return c.count, false, nil
	}
	c.count++
	if c.count >= 3 {
		c.claim = token
		return c.count, true, nil
	}
	return c.count, false, nil
}
func (c *firstOutputCacheStub) CompleteFirstOutputClaim(_ context.Context, _ int64, token string, _ time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.claim == token {
		c.claim = "cooldown"
		c.count = 0
		c.completed++
	}
	return nil
}
func (c *firstOutputCacheStub) ReleaseFirstOutputClaim(_ context.Context, _ int64, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.claim == token {
		c.claim = ""
	}
	return nil
}
func (c *firstOutputCacheStub) ClearFirstOutputProtection(context.Context, int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.claim = ""
	c.count = 0
	return nil
}
func (c *firstOutputCacheStub) AcquireFirstOutputRecovery(_ context.Context, _ int64, token string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.claim != "" && c.claim != "cooldown" {
		return false, nil
	}
	c.claim = token
	return true, nil
}
func (c *firstOutputCacheStub) SetTempUnsched(_ context.Context, _ int64, state *TempUnschedState) error {
	c.state = state
	return nil
}

func (c *firstOutputCacheStub) DeleteTempUnsched(context.Context, int64) error {
	c.state = nil
	return nil
}

type firstOutputRepoStub struct {
	AccountRepository
	account *Account
	fail    bool
	skip    bool
	writes  int
}

func (r *firstOutputRepoStub) GetByID(context.Context, int64) (*Account, error) {
	copy := *r.account
	return &copy, nil
}

func (r *firstOutputRepoStub) ClearTempUnschedulable(context.Context, int64) error {
	r.account.TempUnschedulableUntil = nil
	r.account.TempUnschedulableReason = ""
	return nil
}
func (r *firstOutputRepoStub) ClearModelRateLimits(context.Context, int64) error { return nil }
func (r *firstOutputRepoStub) SetTempUnschedulableIfUnchanged(_ context.Context, _ int64, _ time.Time, until time.Time, reason string) (bool, error) {
	if r.fail {
		return false, errors.New("database unavailable")
	}
	if r.skip {
		return false, nil
	}
	r.writes++
	r.account.TempUnschedulableUntil = &until
	r.account.TempUnschedulableReason = reason
	return true, nil
}

type firstOutputNotifierStub struct{ sent chan *TempUnschedState }

func (n *firstOutputNotifierStub) NotifyFirstOutputBlocked(_ context.Context, _ *Account, _ string, state *TempUnschedState) {
	n.sent <- state
}
func firstOutputFixture() (*RateLimitService, *firstOutputCacheStub, *firstOutputRepoStub) {
	a := &Account{ID: 42, Name: "test", Credentials: map[string]any{"temp_unschedulable_enabled": true, "temp_unschedulable_first_output_enabled": true}}
	c := &firstOutputCacheStub{}
	r := &firstOutputRepoStub{account: a}
	return NewRateLimitService(r, nil, nil, nil, c), c, r
}

func TestFirstOutputProtectionResetNeutralAndTrip(t *testing.T) {
	s, c, r := firstOutputFixture()
	ctx := context.Background()
	n := &firstOutputNotifierStub{sent: make(chan *TempUnschedState, 3)}
	s.SetFirstOutputNotifier(n)
	for _, seconds := range []int{31, 35, 29, 31, 30, 32} {
		s.RecordFirstOutputLatency(ctx, r.account, "model", time.Duration(seconds)*time.Second)
	}
	require.Zero(t, r.writes)
	require.EqualValues(t, 2, c.count)
	require.Len(t, c.signals, 5)
	s.RecordFirstOutputLatency(ctx, r.account, "model", 31*time.Second)
	require.Equal(t, 1, r.writes)
	require.Equal(t, 1, c.completed)
	require.Equal(t, TempUnschedScopeAccount, c.state.Scope)
	require.EqualValues(t, 3, c.state.TriggerCount)
	select {
	case state := <-n.sent:
		require.InDelta(t, 300, state.UntilUnix-time.Now().Unix(), 1)
	case <-time.After(time.Second):
		t.Fatal("missing notification")
	}
	s.RecordFirstOutputLatency(ctx, r.account, "model", time.Second)
	s.RecordFirstOutputLatency(ctx, r.account, "model", 31*time.Second)
	require.Equal(t, 1, r.writes)
	require.Empty(t, n.sent)
}

func TestFirstOutputProtectionRetriesWriteFailureAndSkipsNoop(t *testing.T) {
	for _, skip := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "concurrent change"}[skip], func(t *testing.T) {
			s, c, r := firstOutputFixture()
			r.fail = !skip
			r.skip = skip
			for i := 0; i < 3; i++ {
				s.RecordFirstOutputLatency(context.Background(), r.account, "model", 31*time.Second)
			}
			require.Zero(t, r.writes)
			require.Empty(t, c.claim)
			require.Zero(t, c.completed)
			r.fail = false
			r.skip = false
			s.RecordFirstOutputLatency(context.Background(), r.account, "model", 31*time.Second)
			require.Equal(t, 1, r.writes)
			require.Equal(t, 1, c.completed)
		})
	}
}

func TestFirstOutputObserverDeduplicatesAndIgnoresEarlyFailures(t *testing.T) {
	s, cache, r := firstOutputFixture()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	makeSlow := func() {
		value, _ := c.Get(firstOutputObserverKey)
		o := value.(*firstOutputObservation)
		o.mu.Lock()
		o.start = time.Now().Add(-31 * time.Second)
		o.mu.Unlock()
	}
	finish := beginFirstOutputProtection(context.Background(), c, r.account, "model", true, s)
	makeSlow()
	observeFirstOutput(c)
	observeFirstOutput(c)
	finish()
	finish = beginFirstOutputProtection(context.Background(), c, r.account, "model", true, s)
	makeSlow()
	observeFirstOutput(c)
	finish()
	require.Equal(t, []bool{true}, cache.signals, "slow retries on the same request/account count once")
	finish = beginFirstOutputProtection(context.Background(), c, r.account, "model", true, s)
	observeFirstOutput(c)
	finish()
	require.Equal(t, []bool{true, false}, cache.signals, "a fast retry still resets the window")
	require.Zero(t, cache.count)
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	finish = beginFirstOutputProtection(context.Background(), c2, r.account, "model", true, s)
	finish()
	require.Len(t, cache.signals, 2, "early error is not fast output")
	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	finish = beginFirstOutputProtection(ctx, c3, r.account, "model", true, s)
	cancel()
	finish()
	require.Len(t, cache.signals, 2, "early cancellation does not reset")
}

func TestFirstOutputObserverDeadlineAndLateOutputReportOnce(t *testing.T) {
	s, cache, r := firstOutputFixture()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	finish := beginFirstOutputProtection(context.Background(), c, r.account, "model", true, s)
	defer finish()
	value, _ := c.Get(firstOutputObserverKey)
	o := value.(*firstOutputObservation)
	o.mu.Lock()
	o.start = time.Now().Add(-31 * time.Second)
	o.timer.Reset(time.Millisecond)
	o.mu.Unlock()
	require.Eventually(t, func() bool { cache.mu.Lock(); defer cache.mu.Unlock(); return len(cache.signals) == 1 }, time.Second, time.Millisecond)
	observeFirstOutput(c)
	finish()
	require.Equal(t, []bool{true}, cache.signals)
}

func TestFirstOutputObserverDisabledNonStreamingAndImages(t *testing.T) {
	s, cache, r := firstOutputFixture()
	for _, body := range []string{`{"model":"text","stream":false}`, `{"model":"gemini-3-pro-image","stream":true}`, `{"model":"gpt-image-1","stream":true}`, `{"model":"gpt-text","stream":true,"tools":[{"type":"image_generation"}]}`, `{"model":"gemini","stream":true,"generationConfig":{"responseModalities":["IMAGE"]}}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		finish := beginFirstOutputProtectionBody(context.Background(), c, r.account, []byte(body), s)
		observeFirstOutput(c)
		finish()
		_, exists := c.Get(firstOutputObserverKey)
		require.False(t, exists)
	}
	r.account.Credentials["temp_unschedulable_first_output_enabled"] = false
	s.RecordFirstOutputLatency(context.Background(), r.account, "model", 31*time.Second)
	require.Empty(t, cache.signals)
}

func TestFirstOutputPayloadDistinguishesContentFromMetadata(t *testing.T) {
	for _, payload := range []string{`{"type":"message_start"}`, `{"type":"ping"}`, `{"choices":[{"delta":{"role":"assistant"}}]}`, `{"usage":{"total_tokens":1}}`, `{"candidates":[{"finishReason":"STOP"}]}`} {
		require.False(t, firstOutputPayloadIsMeaningful([]byte(payload)), payload)
	}
	for _, payload := range []string{`{"type":"content_block_delta","delta":{"text":"hi"}}`, `{"type":"content_block_start","content_block":{"type":"tool_use"}}`, `{"choices":[{"delta":{"content":"hi"}}]}`, `{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`, `{"type":"response.output_text.delta","delta":"hi"}`, `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"search"}}]}}]}`} {
		require.True(t, firstOutputPayloadIsMeaningful([]byte(payload)), payload)
	}
}

func TestFirstOutputManualRecoveryClearsCountAndCooldown(t *testing.T) {
	s, cache, repo := firstOutputFixture()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		s.RecordFirstOutputLatency(ctx, repo.account, "model", 31*time.Second)
	}
	require.NotNil(t, repo.account.TempUnschedulableUntil)
	require.NoError(t, s.ClearTempUnschedulable(ctx, repo.account.ID))
	require.Nil(t, repo.account.TempUnschedulableUntil)
	require.Empty(t, cache.claim)
	s.RecordFirstOutputLatency(ctx, repo.account, "model", 31*time.Second)
	require.EqualValues(t, 1, cache.count)
	require.Equal(t, 1, repo.writes)
}

func TestFirstOutputClaudeBridgesObserveClientStreamingOnly(t *testing.T) {
	for _, chat := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			s, cache, repo := firstOutputFixture()
			account := repo.account
			account.Platform = PlatformAnthropic
			account.Type = AccountTypeAPIKey
			account.Credentials["api_key"] = "fixture-key"
			body := `{"model":"claude-sonnet-4-6","input":"hi","stream":STREAM}`
			if chat {
				body = `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":STREAM}`
			}
			streamValue := "false"
			if stream {
				streamValue = "true"
			}
			body = strings.ReplaceAll(body, "STREAM", streamValue)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(namespaceToolAnthropicStream()))}}
			gateway := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream, rateLimitService: s}
			var err error
			if chat {
				_, err = gateway.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
			} else {
				_, err = gateway.ForwardAsResponses(context.Background(), c, account, []byte(body), nil)
			}
			require.NoError(t, err)
			if stream {
				require.Equal(t, []bool{false}, cache.signals)
			} else {
				require.Empty(t, cache.signals)
			}
		}
	}
}
