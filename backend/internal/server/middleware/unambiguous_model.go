package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/gin-gonic/gin"
)

// UnambiguousModel runs independently of the optional group allowlist, before
// composite routing, scheduling, normalization and billing. WebSocket payloads
// are validated at the client-frame reader instead of at the HTTP handshake.
func UnambiguousModel() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}
		queryModels := 0
		for name, values := range c.Request.URL.Query() {
			if strings.EqualFold(name, "model") {
				queryModels += len(values)
			}
		}
		if queryModels > 1 {
			unambiguousModelError(c, http.StatusBadRequest, requestmodel.AmbiguousModelMessage)
			c.Abort()
			return
		}
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
			if err != nil {
				status, message := http.StatusBadRequest, "Failed to read request body"
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					status, message = http.StatusRequestEntityTooLarge, "Request body is too large"
				}
				unambiguousModelError(c, status, message)
				c.Abort()
				return
			}
			requestmodel.ResetRequestBody(c.Request, body)
			if requestmodel.HasAmbiguousBody(c.GetHeader("Content-Type"), body) {
				unambiguousModelError(c, http.StatusBadRequest, requestmodel.AmbiguousModelMessage)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// Keep protocol error shapes without mislabeling a malformed request as a missing model.
func unambiguousModelError(c *gin.Context, status int, message string) {
	path := c.Request.URL.Path
	switch {
	case strings.HasPrefix(path, "/v1beta") || strings.HasPrefix(path, "/antigravity/v1beta"):
		GoogleErrorWriter(c, status, message)
	case strings.Contains(path, "/messages"):
		AnthropicErrorWriter(c, status, message)
	default:
		c.JSON(status, gin.H{"error": gin.H{"type": "invalid_request_error", "message": message}})
	}
}
