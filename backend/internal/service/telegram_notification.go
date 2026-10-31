package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const settingKeyTelegramNotification = "telegram_notification"

type TelegramNotificationSettings struct {
	Enabled            bool   `json:"enabled"`
	ChatID             string `json:"chat_id"`
	BotToken           string `json:"bot_token,omitempty"`
	BotTokenConfigured bool   `json:"bot_token_configured"`
}

type TelegramNotificationService struct {
	repo          SettingRepository
	encryptor     SecretEncryptor
	keyConfigured bool
	client        *http.Client
	baseURL       string
	slots         chan struct{}
}

func NewTelegramNotificationService(repo SettingRepository, encryptor SecretEncryptor, cfg *config.Config) *TelegramNotificationService {
	return &TelegramNotificationService{
		repo: repo, encryptor: encryptor, keyConfigured: cfg != nil && cfg.Totp.EncryptionKeyConfigured,
		client:  &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		baseURL: "https://api.telegram.org", slots: make(chan struct{}, 8),
	}
}

func (s *TelegramNotificationService) load(ctx context.Context) (*TelegramNotificationSettings, error) {
	raw, err := s.repo.GetValue(ctx, settingKeyTelegramNotification)
	if errors.Is(err, ErrSettingNotFound) || (err == nil && strings.TrimSpace(raw) == "") {
		return &TelegramNotificationSettings{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out TelegramNotificationSettings
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, errors.New("invalid Telegram notification settings")
	}
	return &out, nil
}

func (s *TelegramNotificationService) Get(ctx context.Context) (*TelegramNotificationSettings, error) {
	out, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	out.BotTokenConfigured = out.BotToken != ""
	out.BotToken = ""
	return out, nil
}

var telegramBotTokenPattern = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)
var telegramChatIDPattern = regexp.MustCompile(`^(?:-?[0-9]+|@[A-Za-z0-9_]+)$`)

func (s *TelegramNotificationService) Update(ctx context.Context, in TelegramNotificationSettings) (*TelegramNotificationSettings, error) {
	in.ChatID, in.BotToken = strings.TrimSpace(in.ChatID), strings.TrimSpace(in.BotToken)
	if len(in.ChatID) > 128 || (in.ChatID != "" && !telegramChatIDPattern.MatchString(in.ChatID)) {
		return nil, infraerrors.BadRequest("TELEGRAM_CHAT_INVALID", "Telegram Chat ID must be a numeric ID or @channel username")
	}
	if in.BotToken == "" {
		old, err := s.load(ctx)
		if err != nil {
			return nil, err
		}
		in.BotToken = old.BotToken
	} else {
		if len(in.BotToken) > 256 || !telegramBotTokenPattern.MatchString(in.BotToken) {
			return nil, infraerrors.BadRequest("TELEGRAM_TOKEN_INVALID", "Invalid Telegram bot token")
		}
		if !s.keyConfigured || s.encryptor == nil {
			return nil, infraerrors.BadRequest("TELEGRAM_ENCRYPTION_KEY_REQUIRED", "Set a fixed TOTP_ENCRYPTION_KEY before storing the Telegram bot token")
		}
		ciphertext, err := s.encryptor.Encrypt(in.BotToken)
		if err != nil {
			return nil, errors.New("unable to encrypt Telegram token")
		}
		in.BotToken = ciphertext
	}
	if in.Enabled && (in.ChatID == "" || in.BotToken == "") {
		return nil, infraerrors.BadRequest("TELEGRAM_CONFIG_INCOMPLETE", "Telegram bot token and Chat ID are required")
	}
	in.BotTokenConfigured = false
	data, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Set(ctx, settingKeyTelegramNotification, string(data)); err != nil {
		return nil, err
	}
	in.BotTokenConfigured, in.BotToken = in.BotToken != "", ""
	return &in, nil
}

func (s *TelegramNotificationService) NotifyFirstOutputBlocked(ctx context.Context, account *Account, model string, state *TempUnschedState) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return
	}
	settings, err := s.load(ctx)
	if err != nil {
		slog.Warn("telegram_notification_settings_failed")
		return
	}
	if !settings.Enabled {
		return
	}
	if s.encryptor == nil {
		slog.Warn("telegram_notification_encryptor_missing")
		return
	}
	token, err := s.encryptor.Decrypt(settings.BotToken)
	if err != nil {
		slog.Warn("telegram_notification_token_unavailable")
		return
	}
	text := fmt.Sprintf("账号已临时不可调度\n账号：%s (#%d)\n平台：%s\n模型：%s\n原因：首输出超过 30 秒，10 分钟窗口累计 %d 次\n恢复时间：%s\n低于 30 秒的首输出会清零计数与窗口。",
		telegramLabel(account.Name), account.ID, telegramLabel(account.Platform), telegramLabel(model), state.TriggerCount,
		time.Unix(state.UntilUnix, 0).UTC().Format("2006-01-02 15:04:05 UTC"))
	if err := s.send(ctx, token, settings.ChatID, text); err != nil {
		// Never log http.Client errors: their URL contains the bot token.
		slog.Warn("telegram_notification_send_failed", "account_id", account.ID)
	}
}

func telegramLabel(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 160 {
		return string(runes[:160]) + "…"
	}
	return value
}

// send uses Telegram's sendMessage API with plain text (no parse_mode).
// https://core.telegram.org/bots/api#sendmessage
func (s *TelegramNotificationService) send(ctx context.Context, token, chatID, message string) error {
	if !telegramBotTokenPattern.MatchString(token) {
		return errors.New("invalid Telegram token")
	}
	body, err := json.Marshal(map[string]any{"chat_id": chatID, "text": message, "link_preview_options": map[string]bool{"is_disabled": true}})
	if err != nil {
		return errors.New("unable to encode Telegram notification")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("unable to create Telegram request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return errors.New("Telegram request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	var result struct {
		OK bool `json:"ok"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result) != nil || !result.OK {
		return errors.New("Telegram rejected notification")
	}
	return nil
}
