package responseheaders

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// withHideUpstream 保证测试之间不互相污染这个进程级开关。
func withHideUpstream(t *testing.T, enabled bool) {
	t.Helper()
	previous := HideUpstreamEnabled()
	SetHideUpstream(enabled)
	t.Cleanup(func() { SetHideUpstream(previous) })
}

func upstreamRelayResponse() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-Request-Id", "relay-req-abc")
	h.Set("X-Ratelimit-Limit-Requests", "10000")
	h.Set("X-Ratelimit-Remaining-Requests", "9999")
	h.Set("X-Ratelimit-Limit-Tokens", "2000000")
	h.Set("X-Ratelimit-Remaining-Tokens", "1999000")
	h.Set("X-Ratelimit-Reset-Requests", "60s")
	h.Set("X-Ratelimit-Reset-Tokens", "60s")
	h.Set("Retry-After", "30")
	return h
}

func TestHideUpstreamStripsIdentityHeaders(t *testing.T) {
	withHideUpstream(t, true)
	filtered := FilterHeaders(upstreamRelayResponse(),
		CompileHeaderFilter(config.ResponseHeaderConfig{Enabled: true}))

	for _, key := range []string{
		"X-Request-Id",
		"X-Ratelimit-Limit-Requests", "X-Ratelimit-Remaining-Requests",
		"X-Ratelimit-Limit-Tokens", "X-Ratelimit-Remaining-Tokens",
		"X-Ratelimit-Reset-Requests", "X-Ratelimit-Reset-Tokens",
	} {
		if got := filtered.Get(key); got != "" {
			t.Fatalf("%s must not reach the client while hiding, got %q", key, got)
		}
	}
	// 协议必需的头不能一起误伤。
	if got := filtered.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := filtered.Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want 30; clients need it to back off", got)
	}
}

func TestHideUpstreamKeepsIdentityHeadersWhenOff(t *testing.T) {
	withHideUpstream(t, false)
	filtered := FilterHeaders(upstreamRelayResponse(),
		CompileHeaderFilter(config.ResponseHeaderConfig{Enabled: true}))
	if got := filtered.Get("X-Request-Id"); got != "relay-req-abc" {
		t.Fatalf("X-Request-Id = %q, want the upstream value when the toggle is off", got)
	}
}

// 开关是进程级策略，和 security.response_headers.enabled 无关：把自定义过滤关掉
// 不应该静默恢复泄露。
func TestHideUpstreamAppliesEvenWhenCustomFilteringIsDisabled(t *testing.T) {
	withHideUpstream(t, true)
	filter := CompileHeaderFilter(config.ResponseHeaderConfig{Enabled: false})
	if got := FilterHeaders(upstreamRelayResponse(), filter).Get("X-Request-Id"); got != "" {
		t.Fatalf("X-Request-Id = %q, want it stripped regardless of enabled", got)
	}
}

// x-codex-* 不在默认白名单里，但透传路径会强制放行，所以必须能被 IsRemoved 查到。
func TestIsRemovedCoversTheForcedCodexHeaders(t *testing.T) {
	withHideUpstream(t, true)
	filter := CompileHeaderFilter(config.ResponseHeaderConfig{})
	for _, key := range upstreamIdentityHeaders {
		if !filter.IsRemoved(key) {
			t.Fatalf("IsRemoved(%q) = false, want true", key)
		}
	}
	if !filter.IsRemoved("X-Codex-Primary-Used-Percent") {
		t.Fatal("IsRemoved must be case-insensitive")
	}
	if filter.IsRemoved("content-type") {
		t.Fatal("content-type must never be hidden")
	}
	if !(*CompiledHeaderFilter)(nil).IsRemoved("x-request-id") {
		t.Fatal("the policy must hold even for a nil filter")
	}
}

func TestIsRemovedHonoursExplicitForceRemove(t *testing.T) {
	withHideUpstream(t, false)
	filter := CompileHeaderFilter(config.ResponseHeaderConfig{
		Enabled:     true,
		ForceRemove: []string{" X-Request-Id "},
	})
	if !filter.IsRemoved("x-request-id") {
		t.Fatal("force_remove entries must be trimmed, lowercased and queryable")
	}
	if (*CompiledHeaderFilter)(nil).IsRemoved("x-request-id") {
		t.Fatal("a nil filter removes nothing once the policy is off")
	}
}
