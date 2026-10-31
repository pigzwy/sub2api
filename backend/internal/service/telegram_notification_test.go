//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestTelegramSettingsEncryptMaskAndPreserveToken(t *testing.T) {
	repo := newStubSettingRepo()
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	s := NewTelegramNotificationService(repo, reversibleEncryptor{}, cfg)
	out, err := s.Update(context.Background(), TelegramNotificationSettings{Enabled: true, ChatID: "-10012345", BotToken: "123456:secret-test"})
	require.NoError(t, err)
	require.Empty(t, out.BotToken)
	require.True(t, out.BotTokenConfigured)
	var stored TelegramNotificationSettings
	require.NoError(t, json.Unmarshal([]byte(repo.values[settingKeyTelegramNotification]), &stored))
	require.Equal(t, "enc:123456:secret-test", stored.BotToken)
	out, err = s.Update(context.Background(), TelegramNotificationSettings{Enabled: false, ChatID: "-10012345"})
	require.NoError(t, err)
	require.True(t, out.BotTokenConfigured)
	out, err = s.Get(context.Background())
	require.NoError(t, err)
	require.Empty(t, out.BotToken)
	require.True(t, out.BotTokenConfigured)
	s.keyConfigured = false
	_, err = s.Update(context.Background(), TelegramNotificationSettings{BotToken: "123456:replacement"})
	require.Error(t, err)
	_, err = s.Update(context.Background(), TelegramNotificationSettings{Enabled: true, ChatID: "https://example.test"})
	require.Error(t, err)
}

func TestTelegramNotificationPlainTextAndDisabled(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "/bot123456:secret-test/sendMessage", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "-10012345", body["chat_id"])
		require.NotContains(t, body, "parse_mode")
		require.Contains(t, body["text"], "#42")
		require.Contains(t, body["text"], "3 次")
		require.Contains(t, body["text"], "UTC")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	repo := newStubSettingRepo()
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	s := NewTelegramNotificationService(repo, reversibleEncryptor{}, cfg)
	s.baseURL = server.URL
	_, err := s.Update(context.Background(), TelegramNotificationSettings{Enabled: true, ChatID: "-10012345", BotToken: "123456:secret-test"})
	require.NoError(t, err)
	a := &Account{ID: 42, Name: "<test>", Platform: "openai"}
	state := &TempUnschedState{UntilUnix: time.Now().Add(5 * time.Minute).Unix(), TriggerCount: 3}
	s.NotifyFirstOutputBlocked(context.Background(), a, "model", state)
	require.EqualValues(t, 1, calls.Load())
	_, err = s.Update(context.Background(), TelegramNotificationSettings{Enabled: false, ChatID: "-10012345"})
	require.NoError(t, err)
	s.NotifyFirstOutputBlocked(context.Background(), a, "model", state)
	require.EqualValues(t, 1, calls.Load())
}

func TestTelegramSendFailuresDoNotExposeToken(t *testing.T) {
	for _, response := range []struct {
		status int
		body   string
	}{{200, `{"ok":false}`}, {429, `{"ok":false}`}, {200, `not json`}, {302, ``}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://example.test")
			w.WriteHeader(response.status)
			_, _ = w.Write([]byte(response.body))
		}))
		s := NewTelegramNotificationService(nil, nil, nil)
		s.baseURL = server.URL
		err := s.send(context.Background(), "123456:secret-test", "1", "test")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret-test")
		server.Close()
	}
	s := NewTelegramNotificationService(nil, nil, nil)
	s.baseURL = "http://127.0.0.1:1"
	err := s.send(context.Background(), "123456:secret-test", "1", "test")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-test")
	require.LessOrEqual(t, len([]rune(telegramLabel(strings.Repeat("名", 300)))), 161)
}
