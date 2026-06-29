package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type requestInterceptHandlerSettingRepo struct {
	values map[string]string
}

func (r *requestInterceptHandlerSettingRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	if value, ok := r.values[key]; ok {
		return &service.Setting{Key: key, Value: value, UpdatedAt: time.Now()}, nil
	}
	return nil, service.ErrSettingNotFound
}

func (r *requestInterceptHandlerSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}

func (r *requestInterceptHandlerSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *requestInterceptHandlerSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *requestInterceptHandlerSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *requestInterceptHandlerSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *requestInterceptHandlerSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestEvaluateRequestInterceptMarksAuditMocked(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	groupID := int64(123)
	svc := service.NewSettingService(&requestInterceptHandlerSettingRepo{values: map[string]string{}}, nil)
	require.NoError(t, svc.UpdateSettings(ctx, &service.SystemSettings{
		RequestInterceptEnabled:    true,
		RequestInterceptGroupScope: []int64{groupID},
		RequestInterceptRules: []service.RequestInterceptRule{
			{MatchContent: "hi", ResponseContent: "hello"},
		},
	}))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)

	result, ok := evaluateRequestIntercept(c, svc, service.RequestInterceptProtocolOpenAIChat, &groupID, body)

	require.True(t, ok)
	require.Equal(t, "hello", result.Content)
	require.True(t, requestAuditIsMocked(c))
	require.Equal(t, "exact", recorder.Header().Get("X-Sub2API-Request-Intercepted"))
}
