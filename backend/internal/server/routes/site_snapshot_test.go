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

	newEngine := func(enabled bool) *gin.Engine {
		r := gin.New()
		r.GET("/api/v1/internal/site/status", siteSnapshotGuard(enabled), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
		return r
	}
	do := func(r *gin.Engine, remoteAddr string, headers map[string]string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
		req.RemoteAddr = remoteAddr
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	t.Run("disabled_by_default_returns_404_even_from_loopback", func(t *testing.T) {
		require.Equal(t, http.StatusNotFound, do(newEngine(false), "127.0.0.1:54321", nil))
	})

	t.Run("public_peer_returns_404", func(t *testing.T) {
		require.Equal(t, http.StatusNotFound, do(newEngine(true), "203.0.113.10:44321", nil))
	})

	// 来源判断必须基于 TCP 对端：转发头是客户端可以随便写的。
	t.Run("spoofed_forwarding_headers_with_public_peer_return_404", func(t *testing.T) {
		code := do(newEngine(true), "198.51.100.77:55123", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"X-Real-IP":       "127.0.0.1",
		})
		require.Equal(t, http.StatusNotFound, code)
	})

	// 反向代理会带上访客的公网地址，这不该影响判断：对端是本机代理就放行。
	t.Run("local_proxy_carrying_public_client_ip_is_allowed", func(t *testing.T) {
		code := do(newEngine(true), "172.18.0.1:41234", map[string]string{"X-Real-IP": "203.0.113.10"})
		require.Equal(t, http.StatusOK, code)
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
			require.Equal(t, http.StatusOK, do(newEngine(true), addr, nil))
		})
	}

	for _, addr := range []string{"", "not-an-address", "8.8.8.8:53", "[2001:4860:4860::8888]:443"} {
		t.Run("rejected_peer_"+addr, func(t *testing.T) {
			require.Equal(t, http.StatusNotFound, do(newEngine(true), addr, nil))
		})
	}
}
