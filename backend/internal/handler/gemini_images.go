package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// geminiImageForwarder is the minimal surface the async image executor needs to
// dispatch a Gemini image generation. It lets AsyncImageHandler reach
// GatewayHandler.GeminiImages without changing a Wire provider signature — the
// binding is set post-construction in ProvideHandlers.
type geminiImageForwarder interface {
	GeminiImages(c *gin.Context)
}

// GeminiImages serves the OpenAI Images API surface (POST /v1/images/generations
// and /v1/images/edits) for gemini-platform groups. It maps the OpenAI request
// to a Gemini generateContent call, reuses the native /v1beta forwarding path
// (account selection, concurrency, security audit, per-image billing), then maps
// the inline image parts back to the OpenAI Images response shape.
//
// Emitting {data:[{b64_json}]} is what makes the shared async pipeline's
// object-storage offload (ImageResultUploader.Rewrite) transfer Gemini images to
// S3 and hand back presigned URLs exactly like gpt-image and grok — no separate
// storage path is needed.
func (h *GatewayHandler) GeminiImages(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		geminiImagesError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if h.openAIGatewayService == nil {
		geminiImagesError(c, http.StatusServiceUnavailable, "api_error", "image gateway is unavailable")
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			geminiImagesError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		geminiImagesError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		geminiImagesError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	parsed, err := h.openAIGatewayService.ParseOpenAIImagesRequest(c, body)
	if err != nil {
		geminiImagesError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if parsed.Stream {
		geminiImagesError(c, http.StatusBadRequest, "invalid_request_error", "streaming image requests are not supported for gemini")
		return
	}
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		geminiImagesError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}

	requestModel, upstreamModel := geminiImageRequestModels(c, parsed.Model)
	if upstreamModel == "" {
		geminiImagesError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if !service.IsSafeGeminiModelPathSegment(upstreamModel) {
		geminiImagesError(c, http.StatusBadRequest, "invalid_request_error", "Invalid model")
		return
	}

	// Ops parity with the OpenAI/Grok image handlers: record model + request type
	// on the caller's context so ops error logs identify the request. The nested
	// generateContent forward only sets these on its private sub-context.
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(false, false)))

	geminiBody, err := service.BuildGeminiImageGenerateRequest(parsed)
	if err != nil {
		geminiImagesError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	status, header, respBody := h.forwardGeminiGenerateContent(c, upstreamModel, geminiBody)

	// Non-2xx: relay the upstream/gemini error verbatim so callers see the real cause
	// (billing already ran inside the forwarding path).
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		if len(bytes.TrimSpace(respBody)) == 0 {
			geminiImagesError(c, statusOrBadGateway(status), "api_error", "image generation failed")
			return
		}
		contentType := strings.TrimSpace(header.Get("Content-Type"))
		if contentType == "" {
			contentType = "application/json; charset=utf-8"
		}
		c.Data(statusOrBadGateway(status), contentType, respBody)
		return
	}

	openaiBody, _, convErr := service.ConvertGeminiImageResponseToOpenAI(respBody, time.Now().Unix())
	if convErr != nil {
		switch {
		case errors.Is(convErr, service.ErrGeminiImageBlocked):
			geminiImagesError(c, http.StatusBadRequest, "image_generation_blocked", convErr.Error())
		case errors.Is(convErr, service.ErrGeminiImageNoOutput):
			geminiImagesError(c, http.StatusBadGateway, "api_error", "upstream returned no image")
		default:
			geminiImagesError(c, http.StatusBadGateway, "api_error", "invalid upstream image response")
		}
		return
	}

	c.Data(http.StatusOK, "application/json", openaiBody)
}

func geminiImageRequestModels(c *gin.Context, parsedModel string) (requestModel, upstreamModel string) {
	upstreamModel = strings.TrimSpace(parsedModel)
	requestModel = clientRequestedModel(c, upstreamModel)
	if c != nil && c.Request != nil {
		if resolvedModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context()); ok {
			upstreamModel = resolvedModel
		}
	}
	return requestModel, upstreamModel
}

// forwardGeminiGenerateContent invokes the existing native Gemini generateContent
// handler against a synthesized sub-request (translated body + model path param),
// capturing its response with a recorder. This reuses the whole native path —
// account selection, concurrency slots, security audit, and per-image billing —
// instead of re-implementing forwarding. The security-audit "completed" flag (set
// by the async pre-submit moderation, and propagated via the copied gin keys)
// prevents a duplicate audit on the async path.
func (h *GatewayHandler) forwardGeminiGenerateContent(c *gin.Context, model string, geminiBody []byte) (int, http.Header, []byte) {
	// Same sub-request pattern as newAsyncImageContext: Copy() carries the gin
	// keys (APIKey / AuthSubject / Subscription / security-audit flag) and the
	// real engine (ClientIP trusted-proxy config), while the swapped-in recorder
	// writer captures the native handler's response.
	recorder := httptest.NewRecorder()
	recorderCtx, _ := gin.CreateTestContext(recorder)
	sub := c.Copy()
	sub.Writer = recorderCtx.Writer

	// Clone the request to preserve its context (group, force-platform, resolved
	// upstream model, request pricing) and client metadata (RemoteAddr, Host),
	// then override the parts that make it a native generateContent call.
	req := c.Request.Clone(c.Request.Context())
	req.Method = http.MethodPost
	req.URL = &url.URL{Path: "/v1beta/models/" + model + ":generateContent"}
	req.RequestURI = ""
	req.Body = io.NopCloser(bytes.NewReader(geminiBody))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(geminiBody)), nil }
	req.ContentLength = int64(len(geminiBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Del("Content-Length")
	req.Header.Del("Accept") // do not inherit a text/event-stream Accept from the caller
	sub.Request = req
	sub.Params = gin.Params{{Key: "modelAction", Value: model + ":generateContent"}}

	h.GeminiV1BetaModels(sub)

	code := recorder.Code
	if code == 0 {
		code = http.StatusOK
	}
	return code, recorder.Header(), recorder.Body.Bytes()
}

func statusOrBadGateway(status int) int {
	if status <= 0 {
		return http.StatusBadGateway
	}
	return status
}

func geminiImagesError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{"error": gin.H{"type": errType, "message": message}})
}
