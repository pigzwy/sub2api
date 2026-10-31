package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
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
}

type firstOutputAccountRepository interface {
	SetTempUnschedulableIfUnchanged(context.Context, int64, string, time.Time, string) (bool, error)
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

// Recovery generations fence in-flight requests even when Redis is unavailable.
// The marker lives in the account's existing extra JSON, so no schema is needed.
func firstOutputGeneration(account *Account) string {
	if account == nil {
		return ""
	}
	value, _ := account.Extra["first_output_recovery_generation"].(string)
	return value
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
	token := firstOutputGeneration(account) + "/" + uuid.NewString()
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
	if !current.FirstOutputProtectionEnabled() || firstOutputGeneration(current) != firstOutputGeneration(account) {
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
	applied, err := repo.SetTempUnschedulableIfUnchanged(ctx, account.ID, firstOutputGeneration(account), until, string(reason))
	if err != nil {
		slog.Warn("first_output_temp_unsched_failed", "account_id", account.ID)
		return
	}
	if !applied {
		return
	}
	committed = true
	// The repository synchronously refreshes the scheduler snapshot. Do not
	// publish an unfenced second copy of this pause after a concurrent recovery.
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
	started := !o.start.IsZero()
	elapsed := time.Since(o.start)
	if o.timer != nil {
		o.timer.Stop()
	}
	if o.stopCancel != nil {
		o.stopCancel()
	}
	o.mu.Unlock()
	// Early failures/cancellation are not successful fast output.
	if started && (output || elapsed > FirstOutputLimit) {
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
	o := &firstOutputObservation{}
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
	o.stopCancel = context.AfterFunc(ctx, func() { o.finish(false) })
	o.mu.Unlock()
	return func() { o.finish(false); c.Set(firstOutputObserverKey, nil) }
}

type firstOutputContextKey struct{}

func firstOutputProtectionContext(ctx context.Context, c *gin.Context) context.Context {
	if c != nil {
		if value, ok := c.Get(firstOutputObserverKey); ok && value != nil {
			return context.WithValue(ctx, firstOutputContextKey{}, value)
		}
	}
	return ctx
}

// StartFirstOutputProtection starts only when dispatching the real upstream
// operation, after local validation, identity preparation and pool acquisition.
func StartFirstOutputProtection(ctx context.Context) {
	o, _ := ctx.Value(firstOutputContextKey{}).(*firstOutputObservation)
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.done || !o.start.IsZero() {
		return
	}
	o.start = time.Now()
	o.timer = time.AfterFunc(FirstOutputLimit+time.Millisecond, func() { o.finish(false) })
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
		case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta", "response.custom_tool_call_input.delta", "response.refusal.delta":
			return v.Get("delta").String() != ""
		case "response.output_item.added", "response.output_item.done":
			item := v.Get("item")
			if item.Get("type").String() == "function_call" || item.Get("type").String() == "custom_tool_call" {
				return true
			}
			for _, part := range item.Get("content").Array() {
				if firstOutputPartIsMeaningful(part) {
					return true
				}
			}
			for _, part := range item.Get("summary").Array() {
				if firstOutputPartIsMeaningful(part) {
					return true
				}
			}
			return item.Get("encrypted_content").String() != ""
		case "response.content_part.added", "response.content_part.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
			return firstOutputPartIsMeaningful(v.Get("part"))
		case "response.output_text.done", "response.reasoning_text.done", "response.reasoning_summary_text.done":
			return v.Get("text").String() != ""
		case "response.refusal.done":
			return v.Get("refusal").String() != ""
		}
	}
	for _, choice := range v.Get("choices").Array() {
		delta := choice.Get("delta")
		if delta.Get("content").String() != "" || delta.Get("refusal").String() != "" || delta.Get("reasoning_content").String() != "" || delta.Get("reasoning").String() != "" || firstOutputToolIsMeaningful(delta.Get("function_call")) {
			return true
		}
	}
	for _, choice := range v.Get("choices").Array() {
		for _, call := range choice.Get("delta.tool_calls").Array() {
			if call.Get("id").String() != "" || firstOutputToolIsMeaningful(call.Get("function")) {
				return true
			}
		}
	}
	for _, candidate := range v.Get("candidates").Array() {
		for _, part := range candidate.Get("content.parts").Array() {
			if part.Get("text").String() != "" || firstOutputToolIsMeaningful(part.Get("functionCall")) {
				return true
			}
		}
	}
	return false
}

func firstOutputToolIsMeaningful(tool gjson.Result) bool {
	return tool.Get("name").String() != "" || tool.Get("arguments").String() != ""
}

func isMeaningfulFirstOutputAnthropicEvent(event *apicompat.AnthropicStreamEvent) bool {
	if event == nil {
		return false
	}
	if block := event.ContentBlock; block != nil {
		return block.Type == "tool_use" || block.Text != "" || block.Thinking != ""
	}
	if delta := event.Delta; delta != nil {
		return delta.Text != "" || delta.PartialJSON != "" || delta.Thinking != ""
	}
	return false
}

func firstOutputPartIsMeaningful(part gjson.Result) bool {
	return part.Get("text").String() != "" || part.Get("refusal").String() != ""
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

// Production repositories decide atomically; the fallback supports test and
// embedded repositories with the existing account port.
func clearTempUnschedulableForRecovery(ctx context.Context, repo AccountRepository, id int64, manual bool) (bool, error) {
	if conditional, ok := repo.(interface {
		ClearTempUnschedulableForRecovery(context.Context, int64, bool) (bool, error)
	}); ok {
		return conditional.ClearTempUnschedulableForRecovery(ctx, id, manual)
	}
	if !manual {
		account, err := repo.GetByID(ctx, id)
		if err != nil {
			return false, err
		}
		if account != nil && account.isFirstOutputTempUnschedulable() && account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil) {
			return false, nil
		}
	}
	err := repo.ClearTempUnschedulable(ctx, id)
	return err == nil, err
}

func clearObservedTempUnschedulable(ctx context.Context, repo AccountRepository, account *Account) (bool, error) {
	if conditional, ok := repo.(interface {
		ClearTempUnschedulableIfReasonUnchanged(context.Context, int64, string) (bool, error)
	}); ok {
		return conditional.ClearTempUnschedulableIfReasonUnchanged(ctx, account.ID, account.TempUnschedulableReason)
	}
	err := repo.ClearTempUnschedulable(ctx, account.ID)
	return err == nil, err
}
