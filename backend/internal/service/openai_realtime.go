package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"
)

// openai_realtime.go — OpenAI 公开 Realtime 语音 WS 直通（仅 API-Key 账号）。
// 客户端 wss /v1/realtime?model=... ↔ 上游 {base_url}/v1/realtime?...。
// 事件双向原样透传（音频以 base64 承载在 JSON 事件里，无需协议翻译）；
// 上行仅校验 JSON 合法性，下行旁路解析 response.done 交给 onTurn 逐回合落账。
// base_url 为空默认直连官方；配置 base_url 即对接第三方 OpenAI 兼容中转。

const openaiPlatformRealtimeBaseURL = "https://api.openai.com/v1/realtime"

// OpenAIRealtimeTurnUsage 是单个 response.done 抽取出的计费用量。
type OpenAIRealtimeTurnUsage struct {
	// ResponseID 为上游 response.id（resp_...），作为该回合 usage 记录的幂等键。
	ResponseID string
	// ResponseStatus 为 response.status（completed/cancelled/...），仅日志用。
	ResponseStatus string
	Usage          OpenAIUsage
}

// StableOpenAIRealtimeBillingRequestID 保证 realtime 每回合有独立且带前缀的
// 计费 request_id：response.done 缺 id 时生成随机 id，避免同会话多回合在
// usage_billing_dedup / (request_id, api_key_id) 唯一键上互相碰撞。
func StableOpenAIRealtimeBillingRequestID(responseID string) string {
	responseID = strings.TrimSpace(responseID)
	if strings.HasPrefix(responseID, "openai_realtime:") {
		return responseID
	}
	if responseID == "" {
		responseID = generateRequestID()
	}
	return "openai_realtime:" + responseID
}

// BuildOpenAIRealtimeUpstreamURL 由账号 base_url 推导 realtime 的 ws(s) URL。
// 与 Responses 转发同一套约定：base_url 经 validateUpstreamBaseURL 校验后用
// buildOpenAIEndpointURL 拼接 /v1/realtime（base 已带 /v1 时只追加 /realtime）。
// rawQuery 为客户端原始 query，透传后强制覆盖 model。
func (s *OpenAIGatewayService) BuildOpenAIRealtimeUpstreamURL(account *Account, rawQuery, model string) (string, error) {
	if account == nil {
		return "", fmt.Errorf("account is required")
	}
	target := openaiPlatformRealtimeBaseURL
	if base := strings.TrimSpace(account.GetOpenAIBaseURL()); base != "" {
		validated, err := s.validateUpstreamBaseURL(base)
		if err != nil {
			return "", err
		}
		target = buildOpenAIEndpointURL(validated, "/v1/realtime")
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		query = url.Values{}
	}
	if strings.TrimSpace(model) != "" {
		query.Set("model", model)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// ProxyOpenAIRealtime 在客户端 WS 与上游 realtime WS 之间双向直通。
// 返回值为终止转发的第一个错误；正常挂断由调用方用 close-status 判定。
func (s *OpenAIGatewayService) ProxyOpenAIRealtime(
	ctx context.Context,
	client *coderws.Conn,
	account *Account,
	token string,
	upstreamURL string,
	clientHeader http.Header,
	onTurn func(OpenAIRealtimeTurnUsage),
) error {
	if s == nil || client == nil || account == nil {
		return fmt.Errorf("realtime service, client, and account are required")
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return fmt.Errorf("account %d is not an openai apikey account", account.ID)
	}
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	// beta 协议客户端可能携带 OpenAI-Beta: realtime=v1，原样透传。
	if clientHeader != nil {
		if beta := strings.TrimSpace(clientHeader.Get("OpenAI-Beta")); beta != "" {
			headers.Set("OpenAI-Beta", beta)
		}
	}
	account.ApplyHeaderOverrides(headers)

	dialer := s.getOpenAIWSPassthroughDialer()
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstream, _, _, err := dialer.Dial(ctx, upstreamURL, headers, proxyURL)
	if err != nil {
		return err
	}
	defer func() { _ = upstream.Close() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)

	// 上游 → 客户端：旁路解析 response.done 逐回合落账，再原样转发。
	go func() {
		for {
			msg, readErr := upstream.ReadMessage(ctx)
			if readErr != nil {
				errCh <- readErr
				return
			}
			if onTurn != nil {
				if turn, ok := extractOpenAIRealtimeTurnUsage(msg); ok {
					onTurn(turn)
				}
			}
			if writeErr := client.Write(ctx, coderws.MessageText, msg); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
	}()

	// 客户端 → 上游（仅 JSON 事件）。
	go func() {
		for {
			kind, msg, readErr := client.Read(ctx)
			if readErr != nil {
				errCh <- readErr
				return
			}
			if kind != coderws.MessageText && kind != coderws.MessageBinary {
				continue
			}
			var raw json.RawMessage
			if unmarshalErr := json.Unmarshal(msg, &raw); unmarshalErr != nil {
				errCh <- fmt.Errorf("invalid realtime event: %w", unmarshalErr)
				return
			}
			if writeErr := upstream.WriteJSON(ctx, raw); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
	}()

	return <-errCh
}

// extractOpenAIRealtimeTurnUsage 从上游事件中抽取 response.done 的 usage。
// OpenAI Realtime usage 口径：input_tokens 为含缓存总量，
// input_token_details.{text,audio,image}_tokens 为各模态总量（含缓存），
// cached_tokens(+cached_tokens_details) 为缓存子集；
// 映射到 OpenAIUsage 时音频/图片输入记非缓存子集，缓存音频单独记录，
// 与 RecordUsage 的互斥拆桶口径（input - cache_read - cache_creation）对齐。
func extractOpenAIRealtimeTurnUsage(msg []byte) (OpenAIRealtimeTurnUsage, bool) {
	if !gjson.ValidBytes(msg) {
		return OpenAIRealtimeTurnUsage{}, false
	}
	if strings.TrimSpace(gjson.GetBytes(msg, "type").String()) != "response.done" {
		return OpenAIRealtimeTurnUsage{}, false
	}
	response := gjson.GetBytes(msg, "response")
	usage := response.Get("usage")
	if !usage.Exists() {
		return OpenAIRealtimeTurnUsage{}, false
	}
	inputDetails := usage.Get("input_token_details")
	cachedDetails := inputDetails.Get("cached_tokens_details")
	outputDetails := usage.Get("output_token_details")

	cachedAudio := int(cachedDetails.Get("audio_tokens").Int())
	cachedImage := int(cachedDetails.Get("image_tokens").Int())
	turn := OpenAIRealtimeTurnUsage{
		ResponseID:     strings.TrimSpace(response.Get("id").String()),
		ResponseStatus: strings.TrimSpace(response.Get("status").String()),
		Usage: OpenAIUsage{
			InputTokens:          int(usage.Get("input_tokens").Int()),
			OutputTokens:         int(usage.Get("output_tokens").Int()),
			CacheReadInputTokens: int(inputDetails.Get("cached_tokens").Int()),
			AudioInputTokens:     openAIRealtimeNonNegative(int(inputDetails.Get("audio_tokens").Int()) - cachedAudio),
			AudioCacheReadTokens: cachedAudio,
			AudioOutputTokens:    int(outputDetails.Get("audio_tokens").Int()),
			ImageInputTokens:     openAIRealtimeNonNegative(int(inputDetails.Get("image_tokens").Int()) - cachedImage),
		},
	}
	if turn.Usage.InputTokens == 0 && turn.Usage.OutputTokens == 0 {
		return OpenAIRealtimeTurnUsage{}, false
	}
	return turn, true
}

func openAIRealtimeNonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
