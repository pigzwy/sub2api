package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSiteSnapshotGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const secret = "test-snapshot-secret-0123456789abcdef"

	newEngine := func(token string) *gin.Engine {
		r := gin.New()
		r.GET("/api/v1/internal/site/status", siteSnapshotGuard(token), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
		return r
	}

	t.Run("empty_configured_token_returns_404", func(t *testing.T) {
		r := newEngine("")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("wrong_token_returns_404", func(t *testing.T) {
		r := newEngine(secret)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Header.Set(siteSnapshotTokenHeader, "wrong-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("public_peer_with_valid_token_returns_404", func(t *testing.T) {
		r := newEngine(secret)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
		req.RemoteAddr = "203.0.113.10:44321"
		req.Header.Set(siteSnapshotTokenHeader, secret)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("spoofed_xff_loopback_with_public_peer_returns_404", func(t *testing.T) {
		r := newEngine(secret)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
		req.RemoteAddr = "198.51.100.77:55123"
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		req.Header.Set("X-Real-IP", "127.0.0.1")
		req.Header.Set(siteSnapshotTokenHeader, secret)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	for _, addr := range []string{
		"127.0.0.1:54321",
		"[::1]:54321",
		"[::ffff:127.0.0.1]:54321",
		"172.18.0.1:41234",
		"10.0.0.5:41234",
		"192.168.1.20:41234",
	} {
		t.Run("allowed_peer_"+addr, func(t *testing.T) {
			r := newEngine(secret)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
			req.RemoteAddr = addr
			req.Header.Set(siteSnapshotTokenHeader, secret)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})
	}
}
