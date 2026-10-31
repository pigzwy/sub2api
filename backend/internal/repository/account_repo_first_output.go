package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// SetTempUnschedulableIfUnchanged uses the existing state and scheduler
// propagation, but refuses to overwrite an account changed since observation.
// The bool distinguishes a real trip from a concurrent recovery/config change.
func (r *accountRepository) SetTempUnschedulableIfUnchanged(ctx context.Context, id int64, expectedUpdatedAt, until time.Time, reason string) (bool, error) {
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = $1,
		    temp_unschedulable_reason = $2,
		    updated_at = NOW()
		WHERE id = $3 AND updated_at = $4 AND deleted_at IS NULL
		  AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW())
	`, until, reason, id, expectedUpdatedAt)
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
