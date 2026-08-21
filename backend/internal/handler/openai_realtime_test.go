package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRealtimeEnabledForAPIKey(t *testing.T) {
	require.False(t, realtimeEnabledForAPIKey(nil))
	require.False(t, realtimeEnabledForAPIKey(&service.APIKey{}))
	require.False(t, realtimeEnabledForAPIKey(&service.APIKey{
		Group: &service.Group{Platform: service.PlatformOpenAI},
	}))
	require.False(t, realtimeEnabledForAPIKey(&service.APIKey{
		Group: &service.Group{Platform: service.PlatformAnthropic, AllowRealtime: true},
	}))
	require.False(t, realtimeEnabledForAPIKey(&service.APIKey{
		Group: &service.Group{Platform: service.PlatformGrok, AllowRealtime: true},
	}))
	require.True(t, realtimeEnabledForAPIKey(&service.APIKey{
		Group: &service.Group{Platform: service.PlatformOpenAI, AllowRealtime: true},
	}))
	require.True(t, realtimeEnabledForAPIKey(&service.APIKey{
		Group: &service.Group{Platform: service.PlatformComposite, AllowRealtime: true},
	}))
}

func TestRealtimeTargetPlatformAllowedUsesResolvedCompositePlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}
	directContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	directContext.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	require.True(t, realtimeTargetPlatformAllowed(directContext, &service.APIKey{
		Group: &service.Group{Platform: service.PlatformOpenAI},
	}, service.PlatformOpenAI))
	require.True(t, realtimeTargetPlatformAllowed(directContext, &service.APIKey{
		Group: &service.Group{Platform: service.PlatformGrok},
	}, service.PlatformGrok))

	openAIContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	openAIContext.Request = httptest.NewRequest("GET", "/v1/realtime?model=openai-alias", nil)
	openAIContext.Request = openAIContext.Request.WithContext(service.WithResolvedTargetPlatform(openAIContext.Request.Context(), service.PlatformOpenAI))
	require.True(t, realtimeTargetPlatformAllowed(openAIContext, apiKey, service.PlatformOpenAI))
	require.False(t, realtimeTargetPlatformAllowed(openAIContext, apiKey, service.PlatformGrok))

	grokContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	grokContext.Request = httptest.NewRequest("GET", "/realtime?model=grok-alias", nil)
	grokContext.Request = grokContext.Request.WithContext(service.WithResolvedTargetPlatform(grokContext.Request.Context(), service.PlatformGrok))
	require.True(t, realtimeTargetPlatformAllowed(grokContext, apiKey, service.PlatformGrok))
	require.False(t, realtimeTargetPlatformAllowed(grokContext, apiKey, service.PlatformOpenAI))

	unresolvedContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	unresolvedContext.Request = httptest.NewRequest("GET", "/v1/realtime?model=unknown", nil)
	require.False(t, realtimeTargetPlatformAllowed(unresolvedContext, apiKey, service.PlatformOpenAI))
	require.False(t, realtimeTargetPlatformAllowed(unresolvedContext, apiKey, service.PlatformGrok))
}

func TestRealtimeRequestModelsUsesCompositeUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/realtime?model=studio-realtime", nil)
	c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), service.CompositeRouteDecision{
		Matched:        true,
		Source:         service.CompositeRouteSourceExplicit,
		PublicModel:    "studio-realtime",
		TargetPlatform: service.PlatformOpenAI,
		UpstreamModel:  "gpt-realtime-2.1",
	}))

	requestModel, upstreamModel := realtimeRequestModels(c, "gpt-realtime")
	require.Equal(t, "studio-realtime", requestModel)
	require.Equal(t, "gpt-realtime-2.1", upstreamModel)

	directContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	directContext.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	requestModel, upstreamModel = realtimeRequestModels(directContext, "gpt-realtime")
	require.Equal(t, "gpt-realtime", requestModel)
	require.Equal(t, "gpt-realtime", upstreamModel)
}
