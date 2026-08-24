package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIRealtimeTurnUsage(t *testing.T) {
	payload := []byte(`{
		"type": "response.done",
		"response": {
			"id": "resp_abc123",
			"status": "completed",
			"usage": {
				"total_tokens": 1500,
				"input_tokens": 1000,
				"output_tokens": 500,
				"input_token_details": {
					"text_tokens": 300,
					"audio_tokens": 650,
					"image_tokens": 50,
					"cached_tokens": 200,
					"cached_tokens_details": {"text_tokens": 80, "audio_tokens": 100, "image_tokens": 20}
				},
				"output_token_details": {"text_tokens": 120, "audio_tokens": 380}
			}
		}
	}`)

	turn, ok := extractOpenAIRealtimeTurnUsage(payload)
	require.True(t, ok)
	require.Equal(t, "resp_abc123", turn.ResponseID)
	require.Equal(t, "completed", turn.ResponseStatus)
	require.Equal(t, 1000, turn.Usage.InputTokens)
	require.Equal(t, 500, turn.Usage.OutputTokens)
	require.Equal(t, 200, turn.Usage.CacheReadInputTokens)
	// 非缓存音频输入 = 650 - 100
	require.Equal(t, 550, turn.Usage.AudioInputTokens)
	require.Equal(t, 100, turn.Usage.AudioCacheReadTokens)
	require.Equal(t, 380, turn.Usage.AudioOutputTokens)
	// 非缓存图片输入 = 50 - 20
	require.Equal(t, 30, turn.Usage.ImageInputTokens)
}

func TestExtractOpenAIRealtimeTurnUsageIgnoresOtherEvents(t *testing.T) {
	_, ok := extractOpenAIRealtimeTurnUsage([]byte(`{"type":"response.output_audio.delta","delta":"AAAA"}`))
	require.False(t, ok)

	_, ok = extractOpenAIRealtimeTurnUsage([]byte(`{"type":"response.done","response":{"id":"resp_1","status":"cancelled"}}`))
	require.False(t, ok, "response.done without usage must not bill")

	_, ok = extractOpenAIRealtimeTurnUsage([]byte(`{"type":"response.done","response":{"id":"resp_1","usage":{"input_tokens":0,"output_tokens":0}}}`))
	require.False(t, ok, "zero usage must not bill")

	_, ok = extractOpenAIRealtimeTurnUsage([]byte(`not json`))
	require.False(t, ok)
}

func TestStableOpenAIRealtimeBillingRequestID(t *testing.T) {
	require.Equal(t, "openai_realtime:resp_1", StableOpenAIRealtimeBillingRequestID("resp_1"))
	require.Equal(t, "openai_realtime:resp_1", StableOpenAIRealtimeBillingRequestID("openai_realtime:resp_1"))

	generated := StableOpenAIRealtimeBillingRequestID("")
	require.True(t, strings.HasPrefix(generated, "openai_realtime:"))
	require.NotEqual(t, "openai_realtime:", generated)
	require.NotEqual(t, generated, StableOpenAIRealtimeBillingRequestID(""))
}

func TestBuildOpenAIRealtimeUpstreamURL(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	official := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-1"}}
	got, err := svc.BuildOpenAIRealtimeUpstreamURL(official, "model=gpt-realtime-2.1", "gpt-realtime-2.1")
	require.NoError(t, err)
	require.Equal(t, "wss://api.openai.com/v1/realtime?model=gpt-realtime-2.1", got)

	relay := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-1", "base_url": "https://api.tatai.app"}}
	got, err = svc.BuildOpenAIRealtimeUpstreamURL(relay, "model=whatever&foo=bar", "gpt-realtime-2.1")
	require.NoError(t, err)
	require.Equal(t, "wss://api.tatai.app/v1/realtime?foo=bar&model=gpt-realtime-2.1", got)

	relayV1 := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-1", "base_url": "https://relay.example.com/v1"}}
	got, err = svc.BuildOpenAIRealtimeUpstreamURL(relayV1, "", "gpt-realtime")
	require.NoError(t, err)
	require.Equal(t, "wss://relay.example.com/v1/realtime?model=gpt-realtime", got)
}

func TestClassifyOpenAIRealtimeUnavailable(t *testing.T) {
	apikeyWithRealtime := Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "sk-1", "openai_capabilities": []string{"chat_completions", "embeddings", "realtime"},
	}}
	apikeyWithout := Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-2"}}
	oauth := Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "tok"}}
	grok := Account{Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "xai-1"}}

	require.Equal(t, OpenAIRealtimeUnavailableNoAccounts, classifyOpenAIRealtimeUnavailable(nil))
	require.Equal(t, OpenAIRealtimeUnavailableNoAccounts, classifyOpenAIRealtimeUnavailable([]Account{grok}))
	require.Equal(t, OpenAIRealtimeUnavailableNoAPIKeyAccounts, classifyOpenAIRealtimeUnavailable([]Account{oauth}))
	require.Equal(t, OpenAIRealtimeUnavailableCapabilityMissing, classifyOpenAIRealtimeUnavailable([]Account{oauth, apikeyWithout}))
	// 能力齐备但并发上限为 0：占槽恒失败，必须归为配置态而非瞬时态。
	require.Equal(t, OpenAIRealtimeUnavailableConcurrencyZero, classifyOpenAIRealtimeUnavailable([]Account{apikeyWithRealtime}))
	apikeyUsable := apikeyWithRealtime
	apikeyUsable.Concurrency = 3
	require.Equal(t, OpenAIRealtimeUnavailableTransient, classifyOpenAIRealtimeUnavailable([]Account{apikeyWithout, apikeyUsable}))
}

func TestSummarizeOpenAIRealtimePool(t *testing.T) {
	oauth := Account{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "tok"}}
	apikeyWithout := Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 2, Credentials: map[string]any{"api_key": "sk-2"}}
	grok := Account{ID: 9, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "xai-1"}}

	require.Equal(t, "no openai accounts", summarizeOpenAIRealtimePool(nil, nil))
	require.Equal(t, "no openai accounts", summarizeOpenAIRealtimePool([]Account{grok}, nil))
	require.Equal(t, "id=5 type=oauth cap=false conc=0; id=7 type=apikey cap=false conc=2",
		summarizeOpenAIRealtimePool([]Account{oauth, apikeyWithout, grok}, nil))
	require.Equal(t, "id=5 type=oauth cap=false conc=0 sched=stub; id=7 type=apikey cap=false conc=2 sched=stub",
		summarizeOpenAIRealtimePool([]Account{oauth, apikeyWithout}, func(*Account) string { return "stub" }))
}

func TestSummarizeOpenAIRealtimeSchedPoolIncludesEligibilityReason(t *testing.T) {
	newRealtimeAccount := func(id int64) Account {
		return Account{
			ID:          id,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 10,
			Credentials: map[string]any{
				"api_key":             "sk-test",
				"openai_capabilities": []string{"realtime"},
			},
		}
	}

	healthy := newRealtimeAccount(15046)
	quotaExceeded := newRealtimeAccount(15047)
	quotaExceeded.Extra = map[string]any{"quota_limit": 1.0, "quota_used": 1.0}
	modelRateLimited := newRealtimeAccount(15048)
	modelRateLimited.Extra = map[string]any{
		modelRateLimitsKey: map[string]any{
			"gpt-realtime-2.1": map[string]any{
				"rate_limit_reset_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			},
		},
	}
	autoPaused := newRealtimeAccount(15049)
	autoPaused.Extra = map[string]any{
		"codex_5h_used_percent":  95.0,
		"codex_5h_reset_at":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"codex_usage_updated_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
	}
	ctx := withOpenAIQuotaAutoPauseSettings(
		context.Background(),
		OpsOpenAIAccountQuotaAutoPauseSettings{DefaultThreshold5h: 0.9},
	)

	summary := summarizeOpenAIRealtimeSchedPool(
		ctx,
		[]Account{healthy, quotaExceeded, modelRateLimited, autoPaused},
		"gpt-realtime-2.1",
	)
	require.Contains(t, summary, "id=15046 elig=true")
	require.NotContains(t, summary, "id=15046 elig=true elig_reason=")
	// Upstream's eligibility predicate collapses hard quota exhaustion into
	// not_schedulable; the per-window attribution the fork used to emit here was
	// retired when upstream took over the reason API.
	require.Contains(t, summary, "id=15047 elig=false elig_reason=not_schedulable")
	require.Contains(t, summary, "id=15048 elig=false elig_reason=model_rate_limited")
	require.Contains(t, summary, "id=15049 elig=false elig_reason=quota_auto_pause_5h")
	require.Equal(t,
		"empty(账号在调度取数阶段即被丢弃：调度快照未收录或调度阈值拦截)",
		summarizeOpenAIRealtimeSchedPool(context.Background(), nil, "gpt-realtime-2.1"),
	)
}

// Upstream owns the eligibility reason API and reports hard quota exhaustion as
// not_schedulable, so this no longer pins per-window attribution. What it still
// guards is the admission behavior: an account whose total/daily/weekly quota is
// used up must never be selected.
func TestOpenAICompatibleAccountEligibilityRejectsExhaustedQuotaWindows(t *testing.T) {
	now := time.Now().UTC()
	base := Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":             "sk-test",
			"openai_capabilities": []string{"realtime"},
		},
	}
	tests := []struct {
		name   string
		extra  map[string]any
		reason string
	}{
		{
			name:   "total",
			extra:  map[string]any{"quota_limit": 5.0, "quota_used": 5.0},
			reason: "not_schedulable",
		},
		{
			name: "daily",
			extra: map[string]any{
				"quota_daily_limit": 5.0,
				"quota_daily_used":  5.0,
				"quota_daily_start": now.Add(-time.Hour).Format(time.RFC3339),
			},
			reason: "not_schedulable",
		},
		{
			name: "weekly",
			extra: map[string]any{
				"quota_weekly_limit": 5.0,
				"quota_weekly_used":  5.0,
				"quota_weekly_start": now.Add(-time.Hour).Format(time.RFC3339),
			},
			reason: "not_schedulable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := base
			account.Extra = tt.extra
			reason := openAICompatibleAccountEligibilityFailureReason(
				context.Background(), &account, PlatformOpenAI, "gpt-realtime-2.1", false, OpenAIEndpointCapabilityRealtime,
			)
			require.NotEmpty(t, reason, "account must be rejected")
			require.Equal(t, tt.reason, reason)
			require.False(t, isOpenAICompatibleAccountEligibleForRequest(
				context.Background(), &account, PlatformOpenAI, "gpt-realtime-2.1", false, OpenAIEndpointCapabilityRealtime,
			))
		})
	}
}

// 渠道模型限制在账号过滤之前拒绝：nil channelService 时不得误报，
// 保证归因函数在未装配渠道服务的路径上仍回落到账号侧分类。
func TestDiagnoseChannelRestrictionNilChannelServiceIsSafe(t *testing.T) {
	svc := &OpenAIGatewayService{}
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), nil, "gpt-realtime-2.1"))
	groupID := int64(69)
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &groupID, "gpt-realtime-2.1"))
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &groupID, ""))
}

// 调度器本尊的逐账号判定：模型映射白名单不含请求模型时给出 model_not_supported，
// 空映射放行——钉住"配了映射的账号会被静默排除出语音调度"这一机制。
func TestSchedulerVerdictModelNotSupported(t *testing.T) {
	sched, ok := newDefaultOpenAIAccountScheduler(nil, nil).(*defaultOpenAIAccountScheduler)
	require.True(t, ok)
	req := OpenAIAccountScheduleRequest{
		Platform:           PlatformOpenAI,
		RequestedModel:     "gpt-realtime-2.1",
		RequiredTransport:  OpenAIUpstreamTransportHTTPSSE,
		RequiredCapability: OpenAIEndpointCapabilityRealtime,
	}

	mapped := &Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 10, Credentials: map[string]any{
		"api_key":             "sk-1",
		"openai_capabilities": []string{"chat_completions", "embeddings", "realtime"},
		"model_mapping":       map[string]any{"gpt-5.4": "gpt-5.4"},
	}}
	compatible, reason := sched.isAccountRequestCompatibleReason(context.Background(), mapped, req)
	require.False(t, compatible)
	require.Equal(t, "model_not_supported", reason)

	unmapped := &Account{ID: 22, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 10, Credentials: map[string]any{
		"api_key":             "sk-2",
		"openai_capabilities": []string{"chat_completions", "embeddings", "realtime"},
	}}
	compatible, reason = sched.isAccountRequestCompatibleReason(context.Background(), unmapped, req)
	require.True(t, compatible, "空映射=放行所有模型，reason=%s", reason)

	noCap := &Account{ID: 23, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 10, Credentials: map[string]any{"api_key": "sk-3"}}
	compatible, reason = sched.isAccountRequestCompatibleReason(context.Background(), noCap, req)
	require.False(t, compatible)
	require.Equal(t, "capability_mismatch", reason)
}

func TestSupportsOpenAIEndpointCapabilityRealtime(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"openai_capabilities": []string{"chat_completions", "realtime"},
	}}
	require.False(t, oauth.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRealtime), "OAuth 账号不允许 realtime")

	apikeyDefault := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-1"}}
	require.False(t, apikeyDefault.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRealtime), "未显式配置能力集时默认不参与 realtime 调度")

	apikeyWithout := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":             "sk-1",
		"openai_capabilities": []string{"chat_completions", "embeddings"},
	}}
	require.False(t, apikeyWithout.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRealtime))

	apikeyWith := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":             "sk-1",
		"openai_capabilities": []string{"chat_completions", "embeddings", "realtime"},
	}}
	require.True(t, apikeyWith.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRealtime))
	require.True(t, apikeyWith.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions), "勾选 realtime 不影响其他能力")
}

// 利润门与语音路径：Realtime 按 audio token 结算，必须像 Live/Grok 媒体一样
// 抑制利润门。否则分组开启利润控制时，上游声明倍率 ≥ 阈值的账号会在选号阶段
// 被全量否决，对外表现为「没有可用账号」而账号本身完全健康（分组 69 事故）。
func TestProfitControlSuppressedContextSkipsVeto(t *testing.T) {
	rate := 1.0
	account := &Account{ID: 15046, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate}

	// 装门的 ctx：上游 1.00x 高于阈值 0.8 → 否决（未抑制时的行为）。
	gate := &openAIProfitControlGate{groupID: 69, platform: PlatformOpenAI, threshold: 0.8}
	gated := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, gate)
	vetoed, reason := openAIProfitControlVetoReason(gated, account)
	require.True(t, vetoed, "未抑制时应被利润门否决")
	require.NotEmpty(t, reason)

	// 抑制标记会让 withOpenAIProfitControlGate 不装门 → 无门放行。
	svc := &OpenAIGatewayService{}
	groupID := int64(69)
	suppressed := svc.withOpenAIProfitControlGate(WithOpenAIProfitControlSuppressed(context.Background()), &groupID)
	vetoed, _ = openAIProfitControlVetoReason(suppressed, account)
	require.False(t, vetoed, "语音路径抑制利润门后不得否决健康账号")
}

// require_privacy_set 与 Realtime 结构冲突：OpenAI 平台的 IsPrivacySet 要求
// extra.privacy_mode=training_off（仅 OAuth 探测写入），而 Realtime 只调度
// API-Key 账号——两者同时开启时账号必被 privacy_not_set 排除。
// Grok 不受影响（IsPrivacySet 对非 openai 平台恒为 true），这正是
// 「Grok 语音能用、OpenAI 语音打不通」的成因。
func TestPrivacySetGateBlocksAPIKeyRealtimeAccounts(t *testing.T) {
	apikey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-1"}}
	require.False(t, apikey.IsPrivacySet(), "API-Key 账号不可能有 privacy_mode=training_off")

	oauthWithPrivacy := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra: map[string]any{"privacy_mode": PrivacyModeTrainingOff}}
	require.True(t, oauthWithPrivacy.IsPrivacySet())

	grok := &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}
	require.True(t, grok.IsPrivacySet(), "非 openai 平台恒为 true —— Grok 语音不受该开关影响")
}
