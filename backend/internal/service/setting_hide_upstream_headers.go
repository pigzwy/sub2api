package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// 隐藏上游响应头的开关走「请求整流器」设置，读取频率是每个网关请求一次，因此沿用
// 本仓其它运行时设置的做法：进程内 atomic 缓存 + singleflight，热路径零锁。
const (
	hideUpstreamHeadersCacheTTL = 60 * time.Second
	// 读库失败时短缓存快速重试，同时保持默认（隐藏），失败不该导致泄露。
	hideUpstreamHeadersErrorTTL  = 5 * time.Second
	hideUpstreamHeadersDBTimeout = 5 * time.Second
)

type cachedHideUpstreamHeaders struct {
	value     bool
	expiresAt int64 // unix nano
}

var (
	hideUpstreamHeadersCache atomic.Value // *cachedHideUpstreamHeaders
	hideUpstreamHeadersSF    singleflight.Group
)

// HidesUpstreamResponseHeaders 返回是否隐藏会暴露上游身份与配额的响应头。
//
// 未配置过时为 true：多数部署的上游本身是中转或聚合站，把它的请求 ID 与限额数字
// 透给客户端，既暴露转发架构，也会让照着限额排查的人看错数。
func (s *SettingService) HidesUpstreamResponseHeaders(ctx context.Context) bool {
	if s == nil {
		return true
	}
	if cached, ok := hideUpstreamHeadersCache.Load().(*cachedHideUpstreamHeaders); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
		}
	}
	val, _, _ := hideUpstreamHeadersSF.Do("hide_upstream_response_headers", func() (any, error) {
		if cached, ok := hideUpstreamHeadersCache.Load().(*cachedHideUpstreamHeaders); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hideUpstreamHeadersDBTimeout)
		defer cancel()
		settings, err := s.GetRectifierSettings(dbCtx)
		if err != nil {
			slog.Warn("failed to read rectifier settings for hide_upstream_response_headers", "error", err)
			hideUpstreamHeadersCache.Store(&cachedHideUpstreamHeaders{
				value:     true,
				expiresAt: time.Now().Add(hideUpstreamHeadersErrorTTL).UnixNano(),
			})
			return true, nil
		}
		value := settings.HidesUpstreamResponseHeaders()
		hideUpstreamHeadersCache.Store(&cachedHideUpstreamHeaders{
			value:     value,
			expiresAt: time.Now().Add(hideUpstreamHeadersCacheTTL).UnixNano(),
		})
		return value, nil
	})
	if v, ok := val.(bool); ok {
		return v
	}
	return true
}

// InvalidateHideUpstreamResponseHeadersCache 让保存后立即生效，无需等缓存过期。
func InvalidateHideUpstreamResponseHeadersCache() {
	hideUpstreamHeadersCache.Store(&cachedHideUpstreamHeaders{})
}
