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

func TestTelegramRetriesTransientFailuresButNotPermanentRejection(t *testing.T) {
	for _, status := range []int{429, 503, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":1}}`))
					return
				}
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			svc := NewTelegramNotificationService(nil, nil, nil)
			svc.baseURL = server.URL
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := svc.sendWithRetry(ctx, "123456:secret-test", "1", "test")
			if status == 403 {
				require.Error(t, err)
				require.EqualValues(t, 1, calls.Load())
			} else {
				require.NoError(t, err)
				require.EqualValues(t, 2, calls.Load())
			}
		})
	}
}

func TestTelegramRetryAfterHonorsCancellation(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":120}}`))
	}))
	defer server.Close()
	svc := NewTelegramNotificationService(nil, nil, nil)
	svc.baseURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := svc.sendWithRetry(ctx, "123456:secret-test", "1", "test")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-test")
	require.EqualValues(t, 1, calls.Load())
}

func TestTelegramRetriesStopAtFourAttempts(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer server.Close()
	svc := NewTelegramNotificationService(nil, nil, nil)
	svc.baseURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	err := svc.sendWithRetry(ctx, "123456:secret-test", "1", "test")
	require.Error(t, err)
	require.EqualValues(t, 4, calls.Load())
}
