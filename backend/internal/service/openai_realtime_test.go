package service

import (
	"strings"
	"testing"

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
	require.Equal(t, OpenAIRealtimeUnavailableTransient, classifyOpenAIRealtimeUnavailable([]Account{apikeyWithout, apikeyWithRealtime}))
}

func TestSummarizeOpenAIRealtimePool(t *testing.T) {
	oauth := Account{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "tok"}}
	apikeyWithout := Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-2"}}
	grok := Account{ID: 9, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "xai-1"}}

	require.Equal(t, "no openai accounts", summarizeOpenAIRealtimePool(nil))
	require.Equal(t, "no openai accounts", summarizeOpenAIRealtimePool([]Account{grok}))
	require.Equal(t, "id=5 type=oauth cap=false; id=7 type=apikey cap=false",
		summarizeOpenAIRealtimePool([]Account{oauth, apikeyWithout, grok}))
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
