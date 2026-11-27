package routes

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

const siteSnapshotTokenHeader = "X-Site-Snapshot-Token"

// RegisterSiteSnapshotRoutes 注册官网本机定时快照内部接口。
// 不挂 JWT、不挂用户限流、不进审计日志，由 TCP 对端私网/回环守卫 + 共享密钥双重保护。
func RegisterSiteSnapshotRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	snapshotToken string,
) {
	internalSite := v1.Group("/internal/site")
	internalSite.Use(siteSnapshotGuard(snapshotToken))
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

// siteSnapshotGuard 两道访问守卫（任一不通过一律返回 404，不暴露接口存在）：
//  1. 必须已配置非空 SITE_SNAPSHOT_TOKEN；
//  2. TCP 对端地址（c.Request.RemoteAddr，禁止使用可伪造转发头的 c.ClientIP()）必须是回环或私网地址；
//  3. 请求头 X-Site-Snapshot-Token 与配置密钥常数时间匹配。
func siteSnapshotGuard(expectedToken string) gin.HandlerFunc {
	expected := strings.TrimSpace(expectedToken)
	return func(c *gin.Context) {
		if expected == "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if !isLoopbackOrPrivatePeer(c.Request.RemoteAddr) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		provided := c.GetHeader(siteSnapshotTokenHeader)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
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
