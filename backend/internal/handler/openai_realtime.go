package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// OpenAIRealtime 暴露 OpenAI 公开 Realtime 语音 WS（/v1/realtime?model=...）。
// 仅 platform=openai 且开启 allow_realtime 的分组可用；调度只会选中显式
// 勾选 realtime 能力的 API-Key 账号（见 OpenAIEndpointCapabilityRealtime）。
// 每个 response.done 逐回合按 token 落账，连接时长本身不计费。
func (h *OpenAIGatewayHandler) OpenAIRealtime(c *gin.Context) {
	if c == nil || c.Request == nil || !isOpenAIWSUpgradeRequest(c.Request) {
		h.errorResponse(c, http.StatusUpgradeRequired, "invalid_request_error", "WebSocket upgrade required (Upgrade: websocket)")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformOpenAI {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Realtime API is not supported for this platform")
		return
	}
	if !realtimeEnabledForAPIKey(apiKey) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "Realtime is not enabled for this group")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	if !h.ensureResponsesDependencies(c, nil) {
		return
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	// 长连接会话按用户并发占一个槽位，防止单用户无限开会话。
	userRelease, acquired, err := h.concurrencyHelper.TryAcquireUserSlot(
		c.Request.Context(),
		subject.UserID,
		subject.Concurrency,
	)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Realtime concurrency unavailable")
		return
	}
	if !acquired {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Realtime concurrency limit reached")
		return
	}
	defer userRelease()

	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		model = "gpt-realtime"
	}
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.openai_realtime",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.String("model", model),
	)

	selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		c.Request.Context(),
		apiKey.GroupID,
		"",
		"",
		model,
		nil,
		service.OpenAIUpstreamTransportHTTPSSE,
		service.OpenAIEndpointCapabilityRealtime,
		false,
		false,
		false,
	)
	if err != nil || selection == nil || selection.Account == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available realtime accounts")
		return
	}

	var streamStarted bool
	release, slotStatus := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, true, &streamStarted, reqLog)
	if slotStatus != openAISlotAcquireOK {
		return
	}
	defer release()
	account := selection.Account

	token, _, err := h.gatewayService.GetRequestCredential(c.Request.Context(), c, account)
	if err != nil {
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Realtime credential unavailable")
		return
	}
	upstreamURL, err := h.gatewayService.BuildOpenAIRealtimeUpstreamURL(account, c.Request.URL.RawQuery, model)
	if err != nil {
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Invalid realtime upstream configuration")
		return
	}

	conn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	// onTurn 在上游读协程里触发，请求级字段一次性捕获，避免异步读 gin.Context。
	meta := openAIRealtimeUsageMeta{
		userAgent:          c.GetHeader("User-Agent"),
		clientIP:           ip.GetClientIP(c),
		sessionID:          service.ExtractClientSessionID(c),
		inboundEndpoint:    GetInboundEndpoint(c),
		upstreamEndpoint:   GetUpstreamEndpoint(c, account.Platform),
		quotaPlatform:      service.QuotaPlatform(c.Request.Context(), apiKey),
		requestPayloadHash: service.HashUsageRequestPayload([]byte("realtime:" + model)),
		channelUsageFields: clientRequestedUsageFields(c, service.ChannelMappingResult{}, model, ""),
	}
	requestCtx := c.Request.Context()
	started := time.Now()
	onTurn := func(turn service.OpenAIRealtimeTurnUsage) {
		result := &service.OpenAIForwardResult{
			RequestID: service.StableOpenAIRealtimeBillingRequestID(turn.ResponseID),
			Usage:     turn.Usage,
			Model:     model,
			Stream:    true,
			// OpenAIWSMode 令 RecordUsage 直接采用上游 response id 作为幂等键，
			// 避免同会话多回合被请求级 request_id 去重合并成一条。
			OpenAIWSMode: true,
			Duration:     time.Since(started),
		}
		h.submitOpenAIRealtimeUsage(requestCtx, apiKey, account, subscription, meta, result)
	}

	proxyErr := h.gatewayService.ProxyOpenAIRealtime(requestCtx, conn, account, token, upstreamURL, c.Request.Header, onTurn)
	if proxyErr != nil {
		reqLog.Info("openai_realtime.proxy_failed", zap.Error(proxyErr))
		if !isExpectedGrokRealtimeClose(proxyErr) {
			_ = conn.Close(coderws.StatusInternalError, "upstream realtime websocket failed")
			return
		}
	}
}

func realtimeEnabledForAPIKey(apiKey *service.APIKey) bool {
	return apiKey != nil &&
		apiKey.Group != nil &&
		apiKey.Group.Platform == service.PlatformOpenAI &&
		apiKey.Group.AllowRealtime
}

type openAIRealtimeUsageMeta struct {
	userAgent          string
	clientIP           string
	sessionID          string
	inboundEndpoint    string
	upstreamEndpoint   string
	quotaPlatform      string
	requestPayloadHash string
	channelUsageFields service.ChannelUsageFields
}

// submitOpenAIRealtimeUsage 把单回合 usage 交给强制落账队列（money path）。
func (h *OpenAIGatewayHandler) submitOpenAIRealtimeUsage(
	ctx context.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	meta openAIRealtimeUsageMeta,
	result *service.OpenAIForwardResult,
) {
	if h == nil || apiKey == nil || account == nil || result == nil {
		return
	}
	h.submitMandatoryUsageRecordTask(ctx, func(taskCtx context.Context) {
		if err := h.gatewayService.RecordUsage(taskCtx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    meta.inboundEndpoint,
			UpstreamEndpoint:   meta.upstreamEndpoint,
			UserAgent:          meta.userAgent,
			IPAddress:          meta.clientIP,
			RequestPayloadHash: meta.requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      meta.quotaPlatform,
			SessionID:          meta.sessionID,
			ChannelUsageFields: meta.channelUsageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.openai_realtime"),
				zap.Int64("user_id", apiKey.User.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.Int64("account_id", account.ID),
				zap.String("request_id", result.RequestID),
			).Error("openai_realtime.record_usage_failed", zap.Error(err))
		}
	})
}
