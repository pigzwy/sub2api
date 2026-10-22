//go:build unit

package handler

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_PublicContactImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := []byte("\xff\xd8\xff test jpeg")
	image := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	raw := fmt.Sprintf(`{"version":1,"enabled":true,"tabs":[{"id":"service","cards":[{"image":%q}]}]}`, image)
	svc := service.NewSettingService(&settingHandlerPublicRepoStub{values: map[string]string{
		service.SettingKeyContactInfo: raw,
	}}, &config.Config{})
	compact, err := svc.GetCompactPublicSettings(context.Background())
	require.NoError(t, err)
	marker := "/api/v1/settings/public/contact-image/"
	_, rest, found := strings.Cut(compact.ContactInfo, marker)
	require.True(t, found)
	revision, _, found := strings.Cut(rest, `"`)
	require.True(t, found)
	router := gin.New()
	h := NewSettingHandler(svc, "test")
	router.GET(marker+":revision", h.GetPublicContactImage)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, marker+revision, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, imageBytes, recorder.Body.Bytes())
	require.Equal(t, "image/jpeg", recorder.Header().Get("Content-Type"))
	require.Contains(t, recorder.Header().Get("Cache-Control"), "immutable")
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Contains(t, recorder.Header().Get("Content-Security-Policy"), "sandbox")

	request := httptest.NewRequest(http.MethodGet, marker+revision, nil)
	request.Header.Set("If-None-Match", recorder.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	router.ServeHTTP(cached, request)
	require.Equal(t, http.StatusNotModified, cached.Code)
	require.Empty(t, cached.Body.Bytes())

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, marker+"stale", nil))
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Empty(t, missing.Header().Get("Cache-Control"))
}
