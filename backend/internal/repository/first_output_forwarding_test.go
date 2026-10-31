package repository

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise observer propagation through the actual Forward -> HTTPUpstream ->
// response handler chain, including the dispatch-time start hook.
func TestFirstOutputRealHTTPForwardFastRefusalResetsWindow(t *testing.T) {
	server, cache := firstOutputTestCache(t)
	ctx := t.Context()
	for i := 0; i < 2; i++ {
		_, _, err := cache.RecordFirstOutput(ctx, 42, true, "slow")
		require.NoError(t, err)
	}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\",\"content\":[]}}\n\n"+
			"data: {\"type\":\"response.refusal.delta\",\"delta\":\"Cannot comply\"}\n\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
	}))
	defer upstreamServer.Close()
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	upstream := NewHTTPUpstream(cfg)
	limiter := service.NewRateLimitService(nil, nil, cfg, nil, cache)
	gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, limiter, nil, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	account := &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"base_url": upstreamServer.URL, "api_key": "test-key", "temp_unschedulable_enabled": true, "temp_unschedulable_first_output_enabled": true,
	}}
	body := []byte(`{"model":"gpt-5.1","stream":true,"input":"hello"}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	result, err := gateway.Forward(ctx, c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, server.Exists(firstOutputKeys(42)[0]), "fast real upstream output must reset the window")
	transport := upstream.(*httpUpstreamService)
	requireNoUpstreamInFlight(t, transport)
	for _, entry := range transport.clients {
		entry.client.CloseIdleConnections()
	}
}
