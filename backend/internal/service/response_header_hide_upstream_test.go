package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
)

// x-codex-* 在透传路径上绕过白名单强制放行。接中转时这些配额数字来自中转站，
// 必须服从 hide_upstream，否则单靠配置堵不住。
func TestPassthroughResponseHeadersRespectHideUpstream(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "text/event-stream")
	src.Set("X-Request-Id", "relay-req-abc")
	src.Set("X-Codex-Primary-Used-Percent", "42")
	src.Set("X-Codex-Secondary-Window-Minutes", "300")

	responseheaders.SetHideUpstream(true)
	t.Cleanup(func() { responseheaders.SetHideUpstream(false) })

	dst := http.Header{}
	writeOpenAIPassthroughResponseHeaders(dst, src,
		responseheaders.CompileHeaderFilter(config.ResponseHeaderConfig{Enabled: true}))

	for _, key := range []string{"X-Request-Id", "X-Codex-Primary-Used-Percent", "X-Codex-Secondary-Window-Minutes"} {
		if got := dst.Get(key); got != "" {
			t.Fatalf("%s = %q, want it stripped while hiding", key, got)
		}
	}
	if got := dst.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
}

func TestPassthroughResponseHeadersStillForwardCodexQuotaByDefault(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "text/event-stream")
	src.Set("X-Codex-Primary-Used-Percent", "42")

	responseheaders.SetHideUpstream(false)

	dst := http.Header{}
	writeOpenAIPassthroughResponseHeaders(dst, src,
		responseheaders.CompileHeaderFilter(config.ResponseHeaderConfig{Enabled: true}))

	if got := dst.Get("X-Codex-Primary-Used-Percent"); got != "42" {
		t.Fatalf("X-Codex-Primary-Used-Percent = %q, want 42 when the toggle is off", got)
	}
}

// 未配置过的站点必须按「隐藏」处理，否则接了中转的部署会默认泄露。
func TestResponseHeaderPolicyDefaultsToHiding(t *testing.T) {
	if !DefaultResponseHeaderPolicy().HideUpstream {
		t.Fatal("the default policy must hide upstream headers")
	}
}
