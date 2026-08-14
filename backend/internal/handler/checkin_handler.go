package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CheckinHandler 提供每日签到的用户端接口。
//
// 单独成 handler 而不是塞进 UserHandler：签到只依赖签到服务和验证码校验，
// 独立后不必改 UserHandler 的构造签名，也让这块能整体摘除。
type CheckinHandler struct {
	checkinService *service.CheckinService
	authService    *service.AuthService
}

// NewCheckinHandler 构造签到 handler。
func NewCheckinHandler(checkinService *service.CheckinService, authService *service.AuthService) *CheckinHandler {
	return &CheckinHandler{checkinService: checkinService, authService: authService}
}

// checkinRequest 是签到请求体。人机验证关闭时可以整体省略。
type checkinRequest struct {
	CaptchaToken   string `json:"captcha_token"`
	TencentTicket  string `json:"tencent_ticket"`
	TencentRandstr string `json:"tencent_randstr"`
}

// GetCheckin 返回当月签到日历与统计。
// GET /api/v1/user/checkin
func (h *CheckinHandler) GetCheckin(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h.checkinService == nil {
		response.ErrorFrom(c, service.ErrCheckinDisabled)
		return
	}

	snapshot, err := h.checkinService.GetSnapshot(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snapshot)
}

// Checkin 执行一次签到。
// POST /api/v1/user/checkin
func (h *CheckinHandler) Checkin(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h.checkinService == nil {
		response.ErrorFrom(c, service.ErrCheckinDisabled)
		return
	}

	// 请求体可省略：未开启人机验证时前端不需要带任何字段。
	var req checkinRequest
	_ = c.ShouldBindJSON(&req)

	if err := h.verifyCaptchaIfRequired(c, req); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.checkinService.Checkin(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// verifyCaptchaIfRequired 在管理员开启「签到需人机验证」时校验验证码。
//
// 走 VerifyCaptcha 而不是 VerifyActionCaptchaIfEnabled：后者只认腾讯/阿里，
// 站点只配了 Turnstile 时会静默放行，那样这个开关就名不副实了。
func (h *CheckinHandler) verifyCaptchaIfRequired(c *gin.Context, req checkinRequest) error {
	ctx := c.Request.Context()
	if h.checkinService == nil || !h.checkinService.CaptchaRequired(ctx) {
		return nil
	}
	if h.authService == nil {
		return service.ErrServiceUnavailable
	}
	return h.authService.VerifyCaptcha(ctx, service.CaptchaProof{
		TurnstileToken: req.CaptchaToken,
		TencentTicket:  req.TencentTicket,
		TencentRandstr: req.TencentRandstr,
	}, ip.GetClientIP(c))
}
