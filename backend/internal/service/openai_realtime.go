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

// Realtime 选号失败的归因码：503 响应体经 error.code 携带，供下游（如工作台）
// 精确翻译，避免把配置态问题当成瞬时故障提示"稍后再试"。
const (
	// OpenAIRealtimeUnavailableNoAccounts 分组内没有可调度的 openai 账号。
	OpenAIRealtimeUnavailableNoAccounts = "no_schedulable_accounts"
	// OpenAIRealtimeUnavailableNoAPIKeyAccounts 分组内只有 OAuth 账号。
	OpenAIRealtimeUnavailableNoAPIKeyAccounts = "no_apikey_accounts"
	// OpenAIRealtimeUnavailableCapabilityMissing 有 API-Key 账号但均未勾选
	// realtime 端点能力——配置态问题，重试无效。
	OpenAIRealtimeUnavailableCapabilityMissing = "realtime_capability_not_enabled"
	// OpenAIRealtimeUnavailableConcurrencyZero 能力齐备的账号并发上限全部
	// ≤0——占槽 Lua 的 count < maxConcurrency 在 0 上恒假，该账号永远无可用
	// 槽。配置态问题（把账号「并发数」改成 ≥1），重试无效。
	OpenAIRealtimeUnavailableConcurrencyZero = "concurrency_limit_zero"
	// OpenAIRealtimeUnavailableChannelRestricted 分组渠道开启了「限制模型」
	// 且渠道价目表中没有该模型——选号在账号过滤之前即被拒绝（账号本身完全
	// 健康，逐账号判定会显示 sched=ok）。配置态问题，重试无效。
	OpenAIRealtimeUnavailableChannelRestricted = "channel_model_restricted"
	// OpenAIRealtimeUnavailableTransient 存在具备能力且并发上限有效的账号，
	// 本次失败为瞬时态（并发占满/冷却/调度竞争），可重试。
	OpenAIRealtimeUnavailableTransient = "temporarily_unavailable"
)

// classifyOpenAIRealtimeUnavailable 对分组内可调度账号做能力归因。
func classifyOpenAIRealtimeUnavailable(accounts []Account) string {
	openaiCount, apikeyCount, capableCount, usableCount := 0, 0, 0, 0
	for i := range accounts {
		acc := &accounts[i]
		if acc.Platform != PlatformOpenAI {
			continue
		}
		openaiCount++
		if acc.Type == AccountTypeAPIKey {
			apikeyCount++
		}
		if acc.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRealtime) {
			capableCount++
			if acc.Concurrency > 0 {
				usableCount++
			}
		}
	}
	switch {
	case openaiCount == 0:
		return OpenAIRealtimeUnavailableNoAccounts
	case apikeyCount == 0:
		return OpenAIRealtimeUnavailableNoAPIKeyAccounts
	case capableCount == 0:
		return OpenAIRealtimeUnavailableCapabilityMissing
	case usableCount == 0:
		// 能力全过但并发上限全 ≤0：占槽恒失败，是配置态而非瞬时态。
		return OpenAIRealtimeUnavailableConcurrencyZero
	default:
		return OpenAIRealtimeUnavailableTransient
	}
}

// summarizeOpenAIRealtimePool 生成逐账号的排除原因摘要（进网关日志，不进响应体，
// 避免向 key 持有者泄露分组账号构成）。verdict 非 nil 时追加调度器本尊的
// 逐账号判定（sched=ok 或调度器淘汰码），形如：
// "id=5 type=oauth cap=false conc=0 sched=capability_mismatch"。
func summarizeOpenAIRealtimePool(accounts []Account, verdict func(*Account) string) string {
	parts := make([]string, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc.Platform != PlatformOpenAI {
			continue
		}
		entry := fmt.Sprintf("id=%d type=%s cap=%t conc=%d",
			acc.ID, acc.Type, acc.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRealtime), acc.Concurrency)
		if verdict != nil {
			entry += " sched=" + verdict(acc)
		}
		parts = append(parts, entry)
	}
	if len(parts) == 0 {
		return "no openai accounts"
	}
	return strings.Join(parts, "; ")
}

// DiagnoseOpenAIRealtimeUnavailable 在 realtime 选号失败后做一次轻量归因，
// 返回归因码与逐账号摘要（后者供调用方写日志，定位"具体该点哪个账号"）。
// 摘要里的 sched= 字段来自调度器候选过滤的同款判定
// （isAccountRequestCompatibleReason），输出的是调度器自己的淘汰码
// （model_not_supported / channel_upstream_restricted / quota_auto_pause_* /
// proxy_stream_quarantined / capability_mismatch / runtime_blocked /
// shadow_parent_unhealthy / 利润控制码）；sched=ok 仍选号失败 = 占槽竞争或
// 等待超时等瞬时因素。只读一次分组账号列表；任何取数失败都按瞬时态处理。
func (s *OpenAIGatewayService) DiagnoseOpenAIRealtimeUnavailable(ctx context.Context, groupID *int64, model string) (string, string) {
	if s == nil || s.accountRepo == nil || groupID == nil {
		return OpenAIRealtimeUnavailableTransient, ""
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, *groupID)
	if err != nil {
		return OpenAIRealtimeUnavailableTransient, ""
	}
	var verdict func(*Account) string
	if sched, ok := newDefaultOpenAIAccountScheduler(s, nil).(*defaultOpenAIAccountScheduler); ok {
		req := OpenAIAccountScheduleRequest{
			GroupID:            groupID,
			Platform:           PlatformOpenAI,
			RequestedModel:     model,
			RequiredTransport:  OpenAIUpstreamTransportHTTPSSE,
			RequiredCapability: OpenAIEndpointCapabilityRealtime,
		}
		verdict = func(acc *Account) string {
			compatible, reason := sched.isAccountRequestCompatibleReason(ctx, acc, req)
			if compatible {
				return "ok"
			}
			return reason
		}
	}
	summary := summarizeOpenAIRealtimePool(accounts, verdict)

	// 选号真正使用的候选来源与资格判定与上面那份"原始分组账号列表"不是同一套：
	// listSchedulableAccounts 走调度快照 + 调度阈值过滤，
	// isOpenAICompatibleAccountEligibleForRequest 还包含模型级限流、配额自动
	// 暂停、compact 档位等关卡。两份对不上时，差异本身就是根因所在，
	// 因此把选号侧的实况一并落日志（仅失败路径执行一次）。
	if schedAccounts, schedErr := s.listSchedulableAccounts(ctx, groupID, PlatformOpenAI); schedErr != nil {
		summary += fmt.Sprintf(" | sched_pool_err=%v", schedErr)
	} else {
		parts := make([]string, 0, len(schedAccounts))
		for i := range schedAccounts {
			acc := &schedAccounts[i]
			parts = append(parts, fmt.Sprintf("id=%d elig=%t", acc.ID,
				isOpenAICompatibleAccountEligibleForRequest(ctx, acc, PlatformOpenAI, model, false, OpenAIEndpointCapabilityRealtime)))
		}
		if len(parts) == 0 {
			summary += " | sched_pool=empty(账号在调度取数阶段即被丢弃：调度快照未收录或调度阈值拦截)"
		} else {
			summary += " | sched_pool=" + strings.Join(parts, ",")
		}
	}

	// 渠道模型限制在账号过滤之前拒绝，账号侧全部健康也会失败——必须先判，
	// 否则会被误分类成瞬时态。
	if s.checkChannelPricingRestriction(ctx, groupID, model) {
		return OpenAIRealtimeUnavailableChannelRestricted, summary
	}
	return classifyOpenAIRealtimeUnavailable(accounts), summary
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
