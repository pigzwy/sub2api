package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
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
}
