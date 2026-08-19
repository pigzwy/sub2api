//go:build unit

package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIRealtimeTestService(repo *mockAccountRepoForGemini, dialer openAIWSClientDialer) *AccountTestService {
	return &AccountTestService{
		accountRepo:  repo,
		cfg:          &config.Config{},
		grokWSDialer: dialer,
	}
}

func TestAccountTestService_OpenAIRealtimeModeDialsWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 31, Name: "openai-apikey-realtime", Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-upstream-1"},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	dialer := &grokRealtimeTestDialer{
		conn: &grokRealtimeTestConn{msg: []byte(`{"type":"session.created","session":{"id":"sess_rt_1"}}`)},
	}
	svc := newOpenAIRealtimeTestService(repo, dialer)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/31/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeRealtime)

	require.NoError(t, err)
	require.Contains(t, dialer.lastURL, "wss://api.openai.com/v1/realtime")
	require.Contains(t, dialer.lastURL, "model=gpt-realtime")
	require.Equal(t, "Bearer sk-upstream-1", dialer.lastAuth)
	require.Contains(t, rec.Body.String(), "realtime ws handshake ok")
	require.Contains(t, rec.Body.String(), "session.created")
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
	require.Contains(t, rec.Body.String(), `"success":true`)
}

func TestAccountTestService_OpenAIRealtimeCustomBaseURLAndModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 32, Name: "openai-apikey-relay", Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-relay-1", "base_url": "https://relay.example.com/v1"},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	dialer := &grokRealtimeTestDialer{
		conn: &grokRealtimeTestConn{msg: []byte(`{"type":"session.created"}`)},
	}
	svc := newOpenAIRealtimeTestService(repo, dialer)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/32/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "gpt-realtime-2.1", "", AccountTestModeRealtime)

	require.NoError(t, err)
	require.Contains(t, dialer.lastURL, "wss://relay.example.com/v1/realtime")
	require.Contains(t, dialer.lastURL, "model=gpt-realtime-2.1")
}

func TestAccountTestService_OpenAIRealtimeErrorEventFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 33, Name: "openai-apikey-noentitle", Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-upstream-2"},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	dialer := &grokRealtimeTestDialer{
		conn: &grokRealtimeTestConn{msg: []byte(`{"type":"error","error":{"code":"model_not_found","message":"The model does not exist"}}`)},
	}
	svc := newOpenAIRealtimeTestService(repo, dialer)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/33/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "gpt-realtime-9.9", "", AccountTestModeRealtime)

	require.Error(t, err)
	require.Contains(t, rec.Body.String(), `"type":"error"`)
	require.Contains(t, rec.Body.String(), "model_not_found")
}

func TestAccountTestService_OpenAIRealtimeRejectsOAuthAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 34, Name: "openai-oauth", Platform: PlatformOpenAI,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token"},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	dialer := &grokRealtimeTestDialer{}
	svc := newOpenAIRealtimeTestService(repo, dialer)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/34/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeRealtime)

	require.Error(t, err)
	require.Contains(t, rec.Body.String(), "API-Key account")
	require.Empty(t, dialer.lastURL, "OAuth 账号不应发起 realtime 拨号")
}

func TestAccountTestService_OpenAIRealtimeHandshakeFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 35, Name: "openai-apikey-badkey", Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-bad"},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	dialer := &grokRealtimeTestDialer{
		status: 401,
		err:    &openAIWSHandshakeError{Body: []byte(`{"error":{"message":"Incorrect API key"}}`), Err: errors.New("websocket handshake failed")},
	}
	svc := newOpenAIRealtimeTestService(repo, dialer)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/35/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeRealtime)

	require.Error(t, err)
	require.Contains(t, rec.Body.String(), "HTTP 401")
	require.Contains(t, rec.Body.String(), "Incorrect API key")
}

func TestNormalizeAccountTestModeRealtime(t *testing.T) {
	require.Equal(t, AccountTestModeRealtime, normalizeAccountTestMode("realtime"))
	require.Equal(t, AccountTestModeRealtime, normalizeAccountTestMode(" Realtime "))
	require.Equal(t, AccountTestModeDefault, normalizeAccountTestMode("unknown"))
}
