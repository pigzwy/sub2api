package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	FirstOutputLimit       = 30 * time.Second
	FirstOutputWindow      = 10 * time.Minute
	FirstOutputCooldown    = 5 * time.Minute
	FirstOutputThreshold   = 3
	firstOutputObserverKey = "first_output_protection_observer"
	firstOutputRequestKey  = "first_output_protection_request"
)

// FirstOutputProtectionCache is an optional capability of the existing
// temporary-unschedulable cache. Fast output clears only the counting window.
type FirstOutputProtectionCache interface {
	RecordFirstOutput(context.Context, int64, bool, string) (int64, bool, error)
	CompleteFirstOutputClaim(context.Context, int64, string, time.Time) error
	ReleaseFirstOutputClaim(context.Context, int64, string) error
	ClearFirstOutputProtection(context.Context, int64) error
	AcquireFirstOutputRecovery(context.Context, int64, string) (bool, error)
}

type firstOutputAccountRepository interface {
	SetTempUnschedulableIfUnchanged(context.Context, int64, time.Time, time.Time, string) (bool, error)
}

func (s *RateLimitService) clearFirstOutputProtection(ctx context.Context, accountID int64) {
	if cache, ok := s.tempUnschedCache.(FirstOutputProtectionCache); ok {
		if err := cache.ClearFirstOutputProtection(ctx, accountID); err != nil {
			slog.Warn("first_output_recovery_cache_failed", "account_id", accountID)
		}
	}
}

func (a *Account) isFirstOutputTempUnschedulable() bool {
	if a == nil {
		return false
	}
	return gjson.Get(a.TempUnschedulableReason, "matched_keyword").String() == "first_output_timeout"
}

// Serialize manual recovery with a pending trip, so the administrator's clear
// cannot be overwritten by a request that already claimed the third failure.
func (s *RateLimitService) lockFirstOutputRecovery(ctx context.Context, accountID int64) (func(), error) {
	cache, ok := s.tempUnschedCache.(FirstOutputProtectionCache)
	if !ok {
		return func() {}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	token := uuid.NewString()
	for {
		acquired, err := cache.AcquireFirstOutputRecovery(ctx, accountID, token)
		if err != nil {
			return nil, err
		}
		if acquired {
			return func() {
				releaseCtx, releaseCancel := context.WithTimeout(context.Background(), time.Second)
				defer releaseCancel()
				_ = cache.ReleaseFirstOutputClaim(releaseCtx, accountID, token)
			}, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("account protection is updating, retry recovery: %w", ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

type FirstOutputNotifier interface {
	NotifyFirstOutputBlocked(context.Context, *Account, string, *TempUnschedState)
}

func (s *RateLimitService) SetFirstOutputNotifier(notifier FirstOutputNotifier) {
	s.firstOutputNotifier = notifier
}

func (a *Account) FirstOutputProtectionEnabled() bool {
	if a == nil || !a.IsTempUnschedulableEnabled() {
		return false
	}
	enabled, _ := a.Credentials["temp_unschedulable_first_output_enabled"].(bool)
	return enabled
}

// RecordFirstOutputLatency is independent from stream-idle timeouts. Equality
// is intentionally neutral: exactly 30 seconds neither increments nor resets.
func (s *RateLimitService) RecordFirstOutputLatency(ctx context.Context, account *Account, model string, elapsed time.Duration) {
	if s == nil || !account.FirstOutputProtectionEnabled() || elapsed == FirstOutputLimit {
		return
	}
	cache, ok := s.tempUnschedCache.(FirstOutputProtectionCache)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	token := uuid.NewString()
	count, claimed, err := cache.RecordFirstOutput(ctx, account.ID, elapsed > FirstOutputLimit, token)
	if err != nil {
		slog.Warn("first_output_counter_failed", "account_id", account.ID)
		return
	}
	if !claimed {
		return
	}
	committed := false
	defer func() {
		if !committed {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), time.Second)
			defer releaseCancel()
			_ = cache.ReleaseFirstOutputClaim(releaseCtx, account.ID, token)
		}
	}()
	// A previous process may have committed the DB write and died before
	// completing its Redis claim. Re-read under the claim before notifying.
	current, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil || current == nil {
		return
	}
	if !current.FirstOutputProtectionEnabled() {
		return
	}
	now := time.Now()
	if current.TempUnschedulableUntil != nil && now.Before(*current.TempUnschedulableUntil) {
		committed = true
		_ = cache.CompleteFirstOutputClaim(ctx, account.ID, token, *current.TempUnschedulableUntil)
		return
	}
	until := now.Add(FirstOutputCooldown)
	state := &TempUnschedState{
		UntilUnix: until.Unix(), TriggeredAtUnix: now.Unix(), RuleIndex: -1,
		MatchedKeyword: "first_output_timeout", Scope: TempUnschedScopeAccount, Model: model,
		ErrorMessage: "First output exceeded 30 seconds three times within 10 minutes",
		TriggerCount: count, TriggerThreshold: FirstOutputThreshold, TriggerWindowMinutes: 10,
	}
	reason, _ := json.Marshal(state)
	repo, ok := s.accountRepo.(firstOutputAccountRepository)
	if !ok {
		return
	}
	applied, err := repo.SetTempUnschedulableIfUnchanged(ctx, account.ID, current.UpdatedAt, until, string(reason))
	if err != nil {
		slog.Warn("first_output_temp_unsched_failed", "account_id", account.ID)
		return
	}
	if !applied {
		return
	}
	committed = true
	s.notifyAccountSchedulingBlocked(current, until, "first_output_timeout")
	if err := s.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); err != nil {
		slog.Warn("first_output_temp_unsched_cache_failed", "account_id", account.ID)
	}
	if err := cache.CompleteFirstOutputClaim(ctx, account.ID, token, until); err != nil {
		slog.Warn("first_output_claim_complete_failed", "account_id", account.ID)
	}
	if s.firstOutputNotifier != nil {
		// Notification delivery must never hold up the response stream.
		go s.firstOutputNotifier.NotifyFirstOutputBlocked(context.Background(), current, model, state)
	}
}

type firstOutputObservation struct {
	mu         sync.Mutex
	done       bool
	start      time.Time
	timer      *time.Timer
	stopCancel func() bool
	report     func(time.Duration)
}

func (o *firstOutputObservation) finish(output bool) {
	o.mu.Lock()
	if o.done {
		o.mu.Unlock()
		return
	}
	o.done = true
	elapsed := time.Since(o.start)
	if o.timer != nil {
		o.timer.Stop()
	}
	if o.stopCancel != nil {
		o.stopCancel()
	}
	o.mu.Unlock()
	// Early failures/cancellation are not successful fast output.
	if output || elapsed > FirstOutputLimit {
		o.report(elapsed)
	}
}

func beginFirstOutputProtection(ctx context.Context, c *gin.Context, account *Account, model string, stream bool, s *RateLimitService) func() {
	if !stream || s == nil || c == nil || !account.FirstOutputProtectionEnabled() {
		return func() {}
	}
	if _, ok := s.tempUnschedCache.(FirstOutputProtectionCache); !ok {
		return func() {}
	}
	if active, exists := c.Get(firstOutputObserverKey); exists && active != nil {
		return func() {}
	}
	var seen *sync.Map
	if value, ok := c.Get(firstOutputRequestKey); ok {
		seen, _ = value.(*sync.Map)
	}
	if seen == nil {
		seen = &sync.Map{}
		c.Set(firstOutputRequestKey, seen)
	}
	o := &firstOutputObservation{start: time.Now()}
	o.report = func(elapsed time.Duration) {
		// Deduplicate slow increments only. A later fast retry must still reset
		// the account's window, and equality must not consume its slow sample.
		if elapsed > FirstOutputLimit {
			if _, duplicate := seen.LoadOrStore(account.ID, true); duplicate {
				return
			}
		}
		s.RecordFirstOutputLatency(ctx, account, model, elapsed)
	}
	c.Set(firstOutputObserverKey, o)
	o.mu.Lock()
	o.timer = time.AfterFunc(FirstOutputLimit+time.Millisecond, func() { o.finish(false) })
	o.stopCancel = context.AfterFunc(ctx, func() { o.finish(false) })
	o.mu.Unlock()
	return func() { o.finish(false); c.Set(firstOutputObserverKey, nil) }
}

func beginFirstOutputProtectionBody(ctx context.Context, c *gin.Context, account *Account, body []byte, s *RateLimitService) func() {
	if s == nil || c == nil || !account.FirstOutputProtectionEnabled() {
		return func() {}
	}
	model := gjson.GetBytes(body, "model").String()
	stream := gjson.GetBytes(body, "stream").Bool()
	mappedModel := account.GetMappedModel(model)
	if isGeminiImageGenerationIntent(body, model, mappedModel) || IsExplicitImageGenerationIntent("", model, body) || isOpenAIImageGenerationModel(mappedModel) {
		stream = false
	}
	return beginFirstOutputProtection(ctx, c, account, model, stream, s)
}

func observeFirstOutput(c *gin.Context) {
	if c == nil {
		return
	}
	if value, ok := c.Get(firstOutputObserverKey); ok {
		if o, ok := value.(*firstOutputObservation); ok && o != nil {
			o.finish(true)
		}
	}
}

// Metadata, role-only chunks, keepalives and usage frames are not output.
func firstOutputPayloadIsMeaningful(data []byte) bool {
	v := gjson.ParseBytes(data)
	if response := v.Get("response"); response.IsObject() && response.Get("candidates").Exists() {
		v = response
	}
	kind := v.Get("type").String()
	if kind == "content_block_start" {
		block := v.Get("content_block")
		return block.Get("type").String() == "tool_use" || block.Get("text").String() != "" || block.Get("thinking").String() != ""
	}
	if kind == "content_block_delta" {
		delta := v.Get("delta")
		return delta.Get("text").String() != "" || delta.Get("thinking").String() != "" || delta.Get("partial_json").String() != ""
	}
	if strings.HasPrefix(kind, "response.") {
		switch kind {
		case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta":
			return v.Get("delta").String() != ""
		case "response.output_item.added":
			return v.Get("item.type").String() == "function_call"
		}
	}
	for _, choice := range v.Get("choices").Array() {
		delta := choice.Get("delta")
		if delta.Get("content").String() != "" || delta.Get("reasoning_content").String() != "" || delta.Get("reasoning").String() != "" || len(delta.Get("tool_calls").Array()) > 0 || delta.Get("function_call").Exists() {
			return true
		}
	}
	for _, candidate := range v.Get("candidates").Array() {
		for _, part := range candidate.Get("content.parts").Array() {
			if part.Get("text").String() != "" || part.Get("functionCall").Exists() {
				return true
			}
		}
	}
	return false
}

func observeFirstOutputPayload(c *gin.Context, data []byte) {
	if c == nil {
		return
	}
	value, ok := c.Get(firstOutputObserverKey)
	if !ok || value == nil {
		return
	}
	observer, ok := value.(*firstOutputObservation)
	if !ok || observer == nil {
		return
	}
	observer.mu.Lock()
	done := observer.done
	observer.mu.Unlock()
	if done {
		return
	}
	if firstOutputPayloadIsMeaningful(data) {
		observeFirstOutput(c)
	}
}

func observeFirstOutputSSE(c *gin.Context, data []byte) {
	for _, line := range strings.Split(string(data), "\n") {
		if payload, ok := strings.CutPrefix(line, "data:"); ok {
			observeFirstOutputPayload(c, []byte(payload))
		}
	}
}
