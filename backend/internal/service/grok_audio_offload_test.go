//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokVoiceUpstreamStub struct {
	request  *http.Request
	response *http.Response
}

func (s *grokVoiceUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = req
	return s.response, nil
}

func (s *grokVoiceUpstreamStub) DoWithTLS(
	req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func grokVoiceTestAccount() *Account {
	return &Account{
		ID:       11,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://relay.example/v1",
		},
	}
}

func grokVoiceTestContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, target, nil)
	return c, recorder
}

func grokVoiceAudioResponse(requestID string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"audio/mpeg"},
			"X-Request-Id": []string{requestID},
		},
		Body: io.NopCloser(strings.NewReader("audio-bytes")),
	}
}

func TestForwardGrokVoiceArchivesCompletedTTSAudio(t *testing.T) {
	storage := &fakeAudioStorage{}
	upstream := &grokVoiceUpstreamStub{response: grokVoiceAudioResponse("tts-req-1")}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
		audioOffload: newAudioOffloadFixture(storage, "audio/", true),
	}
	c, recorder := grokVoiceTestContext("https://api.example/v1/tts")

	result, err := svc.ForwardGrokVoice(
		context.Background(), c, grokVoiceTestAccount(), "tts", []byte(`{"input":"hello"}`), "application/json",
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "audio-bytes", recorder.Body.String())
	require.NotNil(t, result.AudioUsage)
	require.Equal(t, "tts", result.AudioUsage.Mode)

	require.Eventually(t, func() bool { return storage.count() == 1 }, time.Second, 10*time.Millisecond)
	saved := storage.all()[0]
	require.Equal(t, "audio/2026/08/18/tts-req-1.mp3", saved.key)
	require.Equal(t, "audio/mpeg", saved.contentType)
	require.Equal(t, "audio-bytes", string(saved.data))
}

func TestForwardGrokVoiceKeepsResponseAndBillingWhenStorageFails(t *testing.T) {
	storage := &fakeAudioStorage{err: errors.New("bucket exploded")}
	upstream := &grokVoiceUpstreamStub{response: grokVoiceAudioResponse("tts-req-2")}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
		audioOffload: newAudioOffloadFixture(storage, "audio/", true),
	}
	c, recorder := grokVoiceTestContext("https://api.example/v1/tts")

	result, err := svc.ForwardGrokVoice(
		context.Background(), c, grokVoiceTestAccount(), "tts", []byte(`{"input":"hello"}`), "application/json",
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "audio-bytes", recorder.Body.String())
	require.Equal(t, "audio/mpeg", recorder.Header().Get("Content-Type"))
	require.NotNil(t, result.AudioUsage)
	require.Equal(t, "tts", result.AudioUsage.Mode)
	// 5 runes of input text, billed per million characters.
	require.InDelta(t, 5.0/1_000_000.0, result.AudioUsage.DurationOrUnits, 1e-12)
	require.Equal(t, StableGrokAudioBillingRequestID("tts-req-2"), result.RequestID)

	require.Eventually(t, func() bool { return storage.count() == 1 }, time.Second, 10*time.Millisecond)
}

func TestForwardGrokVoiceDoesNotArchiveSTT(t *testing.T) {
	storage := &fakeAudioStorage{}
	upstream := &grokVoiceUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"text":"hello","duration":3}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
		audioOffload: newAudioOffloadFixture(storage, "audio/", true),
	}
	c, recorder := grokVoiceTestContext("https://api.example/v1/stt")

	_, err := svc.ForwardGrokVoice(
		context.Background(), c, grokVoiceTestAccount(), "stt", []byte(`{"duration_seconds":3}`), "application/json",
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	requireNoAudioSaved(t, storage)
}

func TestForwardGrokVoiceWithoutOffloadServiceStillForwards(t *testing.T) {
	upstream := &grokVoiceUpstreamStub{response: grokVoiceAudioResponse("tts-req-3")}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokVoiceTestContext("https://api.example/v1/tts")

	result, err := svc.ForwardGrokVoice(
		context.Background(), c, grokVoiceTestAccount(), "tts", []byte(`{"input":"hello"}`), "application/json",
	)

	require.NoError(t, err)
	require.Equal(t, "audio-bytes", recorder.Body.String())
	require.NotNil(t, result.AudioUsage)
}
