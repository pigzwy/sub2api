package handler

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const requestAuditCaptureLimit = service.DefaultRequestAuditBodyLimit

type auditCaptureWriter struct {
	gin.ResponseWriter
	buf   []byte
	limit int
}

type requestAuditCaptureDecision struct {
	Enabled        bool
	RetentionHours int
	ScopeUserIDs   []int64
	ScopeGroupIDs  []int64
}

func newAuditCaptureWriter(w gin.ResponseWriter) *auditCaptureWriter {
	return &auditCaptureWriter{ResponseWriter: w, limit: requestAuditCaptureLimit}
}

func (w *auditCaptureWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *auditCaptureWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *auditCaptureWriter) capture(data []byte) {
	if w == nil || len(data) == 0 || len(w.buf) >= w.limit {
		return
	}
	remaining := w.limit - len(w.buf)
	if len(data) > remaining {
		data = data[:remaining]
	}
	w.buf = append(w.buf, data...)
}

func (w *auditCaptureWriter) Captured() []byte {
	if w == nil || len(w.buf) == 0 {
		return nil
	}
	out := make([]byte, len(w.buf))
	copy(out, w.buf)
	return out
}

func (w *auditCaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.Hijack()
}

func (w *auditCaptureWriter) Flush() {
	w.ResponseWriter.Flush()
}

func (w *auditCaptureWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.CloseNotify()
}

func (w *auditCaptureWriter) Pusher() http.Pusher {
	return w.ResponseWriter.Pusher()
}

type requestAuditSubject struct {
	UserID   int64
	APIKeyID int64
	GroupID  *int64
}

type requestAuditSession struct {
	svc       *service.RequestAuditLogService
	decision  requestAuditCaptureDecision
	writer    *auditCaptureWriter
	original  gin.ResponseWriter
	startedAt time.Time
}

func beginRequestAuditCapture(c *gin.Context, svc *service.RequestAuditLogService, settingService *service.SettingService, subject requestAuditSubject) *requestAuditSession {
	if c == nil || svc == nil {
		return nil
	}
	decision := resolveRequestAuditCaptureDecision(c.Request.Context(), settingService, subject.UserID, subject.GroupID)
	if !decision.Enabled {
		return nil
	}
	original := c.Writer
	writer := newAuditCaptureWriter(original)
	c.Writer = writer
	return &requestAuditSession{
		svc:       svc,
		decision:  decision,
		writer:    writer,
		original:  original,
		startedAt: time.Now(),
	}
}

func (s *requestAuditSession) Finish(c *gin.Context, subject requestAuditSubject, platform, endpoint, model string, stream bool, requestBody []byte) {
	if s == nil || s.svc == nil || s.writer == nil || c == nil {
		return
	}
	defer func() {
		if c.Writer == s.writer {
			c.Writer = s.original
		}
	}()

	statusCode := c.Writer.Status()
	durationMs := int(time.Since(s.startedAt).Milliseconds())
	accountID := requestAuditAccountID(c)
	requestID := requestAuditRequestID(c)
	if requestID == "" && c.Request != nil {
		if id, _ := c.Request.Context().Value(ctxkey.RequestID).(string); strings.TrimSpace(id) != "" {
			requestID = strings.TrimSpace(id)
		} else if id, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(id) != "" {
			requestID = strings.TrimSpace(id)
		}
	}
	errMsg := ""
	if len(c.Errors) > 0 {
		errMsg = c.Errors.String()
	}
	recordRequestAuditBestEffort(c.Request.Context(), s.svc, service.RequestAuditLogCreateInput{
		RequestID:      requestID,
		UserID:         subject.UserID,
		APIKeyID:       subject.APIKeyID,
		AccountID:      accountID,
		GroupID:        subject.GroupID,
		RetentionHours: s.decision.RetentionHours,
		ScopeUserIDs:   s.decision.ScopeUserIDs,
		ScopeGroupIDs:  s.decision.ScopeGroupIDs,
		Platform:       platform,
		Endpoint:       endpoint,
		Model:          model,
		Stream:         stream,
		StatusCode:     &statusCode,
		DurationMs:     &durationMs,
		RequestBody:    requestBody,
		ResponseBody:   s.writer.Captured(),
		IsMocked:       requestAuditIsMocked(c),
		MockRuleID:     requestAuditMockRuleID(c),
		ErrorMessage:   errMsg,
	})
}

func requestAuditAccountID(c *gin.Context) *int64 {
	if c == nil {
		return nil
	}
	if v, ok := c.Get("request_audit_account_id"); ok {
		switch id := v.(type) {
		case int64:
			if id > 0 {
				return &id
			}
		case int:
			if id > 0 {
				val := int64(id)
				return &val
			}
		}
	}
	if c.Request != nil {
		if id, ok := c.Request.Context().Value(ctxkey.AccountID).(int64); ok && id > 0 {
			return &id
		}
	}
	return nil
}

func requestAuditRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get("request_audit_request_id"); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func setRequestAuditAccount(c *gin.Context, accountID int64) {
	if c != nil && accountID > 0 {
		c.Set("request_audit_account_id", accountID)
	}
}

func setRequestAuditRequestID(c *gin.Context, requestID string) {
	if c != nil && strings.TrimSpace(requestID) != "" {
		c.Set("request_audit_request_id", strings.TrimSpace(requestID))
	}
}

func setRequestAuditMocked(c *gin.Context, ruleID *int64) {
	if c == nil {
		return
	}
	c.Set("request_audit_is_mocked", true)
	if ruleID != nil && *ruleID > 0 {
		c.Set("request_audit_mock_rule_id", *ruleID)
	}
}

func requestAuditIsMocked(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get("request_audit_is_mocked"); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func requestAuditMockRuleID(c *gin.Context) *int64 {
	if c == nil {
		return nil
	}
	if v, ok := c.Get("request_audit_mock_rule_id"); ok {
		switch id := v.(type) {
		case int64:
			if id > 0 {
				return &id
			}
		case int:
			if id > 0 {
				val := int64(id)
				return &val
			}
		}
	}
	return nil
}

func recordRequestAuditBestEffort(parent context.Context, svc *service.RequestAuditLogService, input service.RequestAuditLogCreateInput) {
	if svc == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
		defer cancel()
		if err := svc.Create(ctx, input); err != nil {
			logger.L().With(zap.String("component", "handler.request_audit")).Warn("request_audit.create_failed", zap.Error(err))
		}
	}()
}

func resolveRequestAuditCaptureDecision(ctx context.Context, settingService *service.SettingService, userID int64, groupID *int64) requestAuditCaptureDecision {
	if settingService == nil {
		return requestAuditCaptureDecision{}
	}
	runtime := settingService.GetRequestAuditRuntime(ctx)
	if !runtime.Enabled {
		return requestAuditCaptureDecision{}
	}
	decision := requestAuditCaptureDecision{
		Enabled:        service.ShouldCaptureRequestAudit(userID, groupID, runtime.UserScope, runtime.GroupScope),
		RetentionHours: runtime.RetentionHours,
		ScopeUserIDs:   runtime.UserScope,
		ScopeGroupIDs:  runtime.GroupScope,
	}
	return decision
}

func reqModelForAudit(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get("request_audit_model"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func streamForAudit(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get("request_audit_stream"); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
