package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// SetTempUnschedulableIfUnchanged uses the existing state and scheduler
// propagation, with a recovery fence independent of last-used bookkeeping.
// The bool distinguishes a real trip from a concurrent recovery or disabled protection.
func (r *accountRepository) SetTempUnschedulableIfUnchanged(ctx context.Context, id int64, expectedGeneration string, until time.Time, reason string) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.SetTempUnschedulableIfUnchanged(ctx, id, expectedGeneration, until, reason)
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
	`, until, reason, id, expectedGeneration)
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
