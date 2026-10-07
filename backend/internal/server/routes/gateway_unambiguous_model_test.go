package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesRejectAmbiguousModelsBeforeDispatch(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformComposite} {
		router := newGatewayRoutesTestRouter(platform)
		for _, path := range []string{
			"/v1/responses", "/responses", "/backend-api/codex/responses",
			"/v1/responses/compact", "/responses/compact", "/backend-api/codex/responses/compact",
			"/v1/chat/completions", "/chat/completions", "/v1/messages", "/messages/count_tokens",
			"/v1/embeddings", "/v1/images/generations", "/v1/tts",
			"/v1/live", "/backend-api/codex/realtime/calls", "/antigravity/v1/messages",
		} {
			t.Run(platform+path, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"first","\u006dodel":"last","messages":[{"role":"user","content":"hello"}]}`))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Contains(t, w.Body.String(), "duplicate model")
			})
		}
	}
}
