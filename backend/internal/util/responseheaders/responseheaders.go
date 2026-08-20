package responseheaders

import (
	"net/http"
	"strings"

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

// IsRemoved 供那些在过滤之后又单独回写响应头的路径查询，避免绕过 force_remove。
func (f *CompiledHeaderFilter) IsRemoved(key string) bool {
	if f == nil {
		return false
	}
	_, removed := f.forceRemove[strings.ToLower(strings.TrimSpace(key))]
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
	// HideUpstream 刻意不受 Enabled 约束：它是一个「别暴露自己是中转」的独立诉求，
	// 若也跟着 Enabled 走，关掉自定义过滤就会静默恢复泄露，正是最容易踩空的地方。
	if cfg.HideUpstream {
		for _, key := range upstreamIdentityHeaders {
			forceRemove[key] = struct{}{}
		}
	}
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
	for key, values := range src {
		lower := strings.ToLower(key)
		if _, blocked := filter.forceRemove[lower]; blocked {
			continue
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
