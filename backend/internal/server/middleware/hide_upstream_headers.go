package middleware

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
)

// HideUpstreamResponseHeaders 在每个请求前把「隐藏上游响应头」策略刷进过滤器。
//
// 响应头白名单在启动时编译一次并注入各网关服务，改不动；策略又必须保存即生效，
// 所以在这里用带缓存的设置值刷新进程级开关。放在最外层而不是逐个改 40 多处写头
// 的调用点：既避免大面积改动上游文件，也让以后新增的响应路径自动受管。
//
// resolve 内部是 60 秒缓存 + singleflight，每请求调用一次的代价可忽略。
func HideUpstreamResponseHeaders(resolve func(context.Context) bool) gin.HandlerFunc {
	if resolve == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		ctx := context.Background()
		if c.Request != nil {
			ctx = c.Request.Context()
		}
		responseheaders.SetHideUpstream(resolve(ctx))
		c.Next()
	}
}
