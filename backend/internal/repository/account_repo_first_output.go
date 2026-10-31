package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// SetTempUnschedulableIfUnchanged uses the existing state and scheduler
// propagation, with a recovery fence independent of last-used bookkeeping.
// The bool distinguishes a real trip from a concurrent recovery or disabled protection.
func (r *accountRepository) SetTempUnschedulableIfUnchanged(ctx context.Context, id int64, expectedGeneration string, config service.FirstOutputProtectionConfig, until time.Time, reason string) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.SetTempUnschedulableIfUnchanged(ctx, id, expectedGeneration, config, until, reason)
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = $1,
		    temp_unschedulable_reason = $2,
		    updated_at = NOW()
		WHERE id = $3 AND COALESCE(extra->>'first_output_recovery_generation', '') = $4 AND deleted_at IS NULL
		  AND credentials->'temp_unschedulable_enabled' = 'true'::jsonb
		  AND credentials->'temp_unschedulable_first_output_enabled' = 'true'::jsonb
		  AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW())
 AND `+firstOutputConfigValueSQL(service.FirstOutputLimitSecondsKey, 30, service.FirstOutputMaxLimitSeconds)+` = $5
 AND `+firstOutputConfigValueSQL(service.FirstOutputCooldownMinutesKey, 5, service.FirstOutputMaxMinutes)+` = $6
 AND `+firstOutputConfigValueSQL(service.FirstOutputWindowMinutesKey, 10, service.FirstOutputMaxMinutes)+` = $7
 `, until, reason, id, expectedGeneration, config.LimitSeconds, config.CooldownMinutes, config.WindowMinutes)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return false, err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue first-output pause failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return true, nil
}

// Token refresh may clear the auth-related state it observed, but must not
// erase a newer pause applied by another request in the meantime.
func (r *accountRepository) ClearTempUnschedulableIfReasonUnchanged(ctx context.Context, id int64, expectedReason string) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.ClearTempUnschedulableIfReasonUnchanged(ctx, id, expectedReason)
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts SET temp_unschedulable_until = NULL,
		  temp_unschedulable_reason = NULL, updated_at = NOW()
		WHERE id = $1 AND COALESCE(temp_unschedulable_reason, '') = $2 AND deleted_at IS NULL
	`, id, expectedReason)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return false, err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue token refresh recovery failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return true, nil
}

// ClearTempUnschedulableForRecovery atomically distinguishes manual recovery
// from health/quota recovery. Automatic recovery cannot shorten a live cooldown.
func (r *accountRepository) ClearTempUnschedulableForRecovery(ctx context.Context, id int64, manual bool) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.ClearTempUnschedulableForRecovery(ctx, id, manual)
	}
	result, err := r.sql.ExecContext(ctx, `
 UPDATE accounts SET temp_unschedulable_until = NULL, temp_unschedulable_reason = NULL,
 extra = CASE WHEN $2 THEN COALESCE(extra, '{}'::jsonb) || jsonb_build_object('first_output_recovery_generation', $3::text) ELSE extra END,
 updated_at = NOW()
 WHERE id = $1 AND deleted_at IS NULL
 AND ($2 OR temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW()
      OR COALESCE(temp_unschedulable_reason, '') !~ '"matched_keyword"[[:space:]]*:[[:space:]]*"first_output_timeout"')
 `, id, manual, uuid.NewString())
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return false, err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue account recovery failed: account=%d err=%v", id, err)
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return true, nil
}

// Mirror the account config reader without casting malformed legacy JSON values.
// Key names and bounds are application constants, never request input.
func firstOutputConfigValueSQL(key string, fallback, maximum int) string {
	return firstOutputConfigValueFromSQL("credentials", key, fallback, maximum)
}

func firstOutputConfigValueFromSQL(source, key string, fallback, maximum int) string {
	return fmt.Sprintf(`(CASE WHEN jsonb_typeof((%[4]s)->'%[1]s') = 'number' THEN
  CASE WHEN ((%[4]s)->>'%[1]s')::numeric BETWEEN 1 AND %[3]d
   AND trunc(((%[4]s)->>'%[1]s')::numeric) = ((%[4]s)->>'%[1]s')::numeric
   THEN ((%[4]s)->>'%[1]s')::numeric ELSE %[2]d END ELSE %[2]d END)`, key, fallback, maximum, source)
}

// Relevant account edits reset the recovery fence as well. This prevents an
// A -> B -> A settings change from reviving A's old counters or in-flight trips.
func firstOutputConfigChangedSQL(previous, next string) string {
	enabled := func(source string) string {
		return fmt.Sprintf("COALESCE((%[1]s)->'temp_unschedulable_enabled' = 'true'::jsonb AND (%[1]s)->'temp_unschedulable_first_output_enabled' = 'true'::jsonb, false)", source)
	}
	clauses := []string{enabled(previous) + " IS DISTINCT FROM " + enabled(next)}
	defaults := service.DefaultFirstOutputProtectionConfig()
	for _, field := range []struct {
		key               string
		fallback, maximum int
	}{
		{service.FirstOutputLimitSecondsKey, defaults.LimitSeconds, service.FirstOutputMaxLimitSeconds},
		{service.FirstOutputCooldownMinutesKey, defaults.CooldownMinutes, service.FirstOutputMaxMinutes},
		{service.FirstOutputWindowMinutesKey, defaults.WindowMinutes, service.FirstOutputMaxMinutes},
	} {
		clauses = append(clauses, firstOutputConfigValueFromSQL(previous, field.key, field.fallback, field.maximum)+" IS DISTINCT FROM "+firstOutputConfigValueFromSQL(next, field.key, field.fallback, field.maximum))
	}
	return "(" + strings.Join(clauses, " OR ") + ")"
}
