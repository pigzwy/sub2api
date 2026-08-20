package responseheaders

import (
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// defaultAllowed 定义允许透传的响应头白名单
// 注意：以下头部由 Go HTTP 包自动处理，不应手动设置：
//   - content-length: 由 ResponseWriter 根据实际写入数据自动设置
//   - transfer-encoding: 由 HTTP 库根据需要自动添加/移除
//   - connection: 由 HTTP 库管理连接复用
var defaultAllowed = map[string]struct{}{
	"content-type":                   {},
	"content-encoding":               {},
	"content-language":               {},
	"cache-control":                  {},
	"etag":                           {},
	"last-modified":                  {},
	"expires":                        {},
	"vary":                           {},
	"date":                           {},
	"x-request-id":                   {},
	"x-ratelimit-limit-requests":     {},
	"x-ratelimit-limit-tokens":       {},
	"x-ratelimit-remaining-requests": {},
	"x-ratelimit-remaining-tokens":   {},
	"x-ratelimit-reset-requests":     {},
	"x-ratelimit-reset-tokens":       {},
	"retry-after":                    {},
	"location":                       {},
	"www-authenticate":               {},
}

// hopByHopHeaders 是跳过的 hop-by-hop 头部，这些头部由 HTTP 库自动处理
var hopByHopHeaders = map[string]struct{}{
	"content-length":    {},
	"transfer-encoding": {},
	"connection":        {},
}

// upstreamIdentityHeaders 是白名单里会暴露上游身份与配额状态的响应头。
//
// 当上游本身也是一台中转（sub2api / new-api 之类）时，把它们原样透传，等于直接
// 告诉客户端"这里是转发的"，而且给出的请求 ID 与限额数字来自中转站，既暴露架构
// 又会误导排查。hide_upstream 打开后一律剥掉。
//
// x-codex-* 在透传路径上被强制放行（见 writeOpenAIPassthroughResponseHeaders），
// 所以必须一并登记，否则单靠 force_remove 堵不住。
// hideUpstream 是进程级策略，由后台「请求整流器」的开关驱动。
//
// 过滤器在启动时编译一次并注入到各个网关服务，改不动；而这个开关必须保存即生效，
// 因此用一个原子量承载。策略本身是全站级的（不区分分组/请求），用进程级状态表达
// 是贴切的，不是把请求态藏进全局。
var hideUpstream atomic.Bool

// SetHideUpstream 由中间件在每个请求前用带缓存的设置值刷新，因此未收到保存事件的
// 副本也会在一个缓存周期内自行收敛。
func SetHideUpstream(enabled bool) { hideUpstream.Store(enabled) }

// HideUpstreamEnabled 供测试与诊断读取当前策略。
func HideUpstreamEnabled() bool { return hideUpstream.Load() }

var upstreamIdentityHeaderSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(upstreamIdentityHeaders))
	for _, key := range upstreamIdentityHeaders {
		set[key] = struct{}{}
	}
	return set
}()

var upstreamIdentityHeaders = []string{
	"x-request-id",
	"x-ratelimit-limit-requests",
	"x-ratelimit-limit-tokens",
	"x-ratelimit-remaining-requests",
	"x-ratelimit-remaining-tokens",
	"x-ratelimit-reset-requests",
	"x-ratelimit-reset-tokens",
	"x-codex-primary-used-percent",
	"x-codex-primary-reset-after-seconds",
	"x-codex-primary-window-minutes",
	"x-codex-secondary-used-percent",
	"x-codex-secondary-reset-after-seconds",
	"x-codex-secondary-window-minutes",
	"x-codex-primary-over-secondary-limit-percent",
}

type CompiledHeaderFilter struct {
	allowed     map[string]struct{}
	forceRemove map[string]struct{}
}

// IsRemoved 供那些在过滤之后又单独回写响应头的路径查询，避免绕过 force_remove
// 与隐藏上游开关。
func (f *CompiledHeaderFilter) IsRemoved(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if hideUpstream.Load() {
		if _, hidden := upstreamIdentityHeaderSet[normalized]; hidden {
			return true
		}
	}
	if f == nil {
		return false
	}
	_, removed := f.forceRemove[normalized]
	return removed
}

var defaultCompiledHeaderFilter = CompileHeaderFilter(config.ResponseHeaderConfig{})

func CompileHeaderFilter(cfg config.ResponseHeaderConfig) *CompiledHeaderFilter {
	allowed := make(map[string]struct{}, len(defaultAllowed)+len(cfg.AdditionalAllowed))
	for key := range defaultAllowed {
		allowed[key] = struct{}{}
	}
	// 关闭时只使用默认白名单，additional/force_remove 不生效
	if cfg.Enabled {
		for _, key := range cfg.AdditionalAllowed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				continue
			}
			allowed[normalized] = struct{}{}
		}
	}

	forceRemove := map[string]struct{}{}
	if cfg.Enabled {
		for _, key := range cfg.ForceRemove {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				continue
			}
			forceRemove[normalized] = struct{}{}
		}
	}

	return &CompiledHeaderFilter{
		allowed:     allowed,
		forceRemove: forceRemove,
	}
}

func FilterHeaders(src http.Header, filter *CompiledHeaderFilter) http.Header {
	if filter == nil {
		filter = defaultCompiledHeaderFilter
	}

	filtered := make(http.Header, len(src))
	hidden := hideUpstream.Load()
	for key, values := range src {
		lower := strings.ToLower(key)
		if _, blocked := filter.forceRemove[lower]; blocked {
			continue
		}
		if hidden {
			if _, identity := upstreamIdentityHeaderSet[lower]; identity {
				continue
			}
		}
		if _, ok := filter.allowed[lower]; !ok {
			continue
		}
		// 跳过 hop-by-hop 头部，这些由 HTTP 库自动处理
		if _, isHopByHop := hopByHopHeaders[lower]; isHopByHop {
			continue
		}
		for _, value := range values {
			filtered.Add(key, value)
		}
	}
	return filtered
}

func WriteFilteredHeaders(dst http.Header, src http.Header, filter *CompiledHeaderFilter) {
	filtered := FilterHeaders(src, filter)
	for key, values := range filtered {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
