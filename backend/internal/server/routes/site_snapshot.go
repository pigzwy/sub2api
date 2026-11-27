package routes

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterSiteSnapshotRoutes 注册官网状态页的数据接口。
//
// 访客请求由官网的反向代理（本机 Caddy）转发过来，接口自带缓存，返回的也只有脱敏后的公开数据，
// 所以不挂 JWT、不挂用户限流、不进审计日志。两道开关：
//  1. 必须显式开启（SITE_STATUS_ENABLED=true），默认关闭；
//  2. TCP 对端必须是回环或私网地址，即只接受本机反向代理转来的请求——
//     直连网关端口进来的公网地址一律拒绝。
//
// 任一不满足都返回 404，不暴露接口存在。
func RegisterSiteSnapshotRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	enabled bool,
) {
	internalSite := v1.Group("/internal/site")
	internalSite.Use(siteSnapshotGuard(enabled))
	{
		internalSite.GET("/status", func(c *gin.Context) {
			if h == nil || h.SiteSnapshot == nil {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			h.SiteSnapshot.GetStatus(c)
		})
	}
}

// siteSnapshotGuard 见 RegisterSiteSnapshotRoutes。
// 来源地址取 c.Request.RemoteAddr，不能用 c.ClientIP()：后者会读可伪造的转发头。
func siteSnapshotGuard(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled || !isLoopbackOrPrivatePeer(c.Request.RemoteAddr) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Next()
	}
}

func isLoopbackOrPrivatePeer(remoteAddr string) bool {
	raw := strings.TrimSpace(remoteAddr)
	if raw == "" {
		return false
	}
	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate()
}
