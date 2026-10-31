//go:build integration

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func (s *AccountRepoSuite) TestFirstOutputRecoveryFenceAndAutomaticCooldown() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "first-output-regression", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"temp_unschedulable_enabled": true, "temp_unschedulable_first_output_enabled": true},
	})
	// Routine last-used bookkeeping must not drop the third-hit pause.
	s.Require().NoError(s.repo.BatchUpdateLastUsed(s.ctx, map[int64]time.Time{account.ID: time.Now()}))
	until := time.Now().Add(5 * time.Minute)
	reason := `{"matched_keyword": "first_output_timeout", "scope": "account"}`
	applied, err := s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, "", until, reason)
	s.Require().NoError(err)
	s.Require().True(applied)
	// Pretty-printed JSON is still protected from automated recovery.
	cleared, err := s.repo.ClearTempUnschedulableForRecovery(s.ctx, account.ID, false)
	s.Require().NoError(err)
	s.Require().False(cleared)
	current, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(current.TempUnschedulableUntil)
	// The legacy admin single/bulk clear path advances the same recovery fence.
	admin := service.NewAdminService(nil, nil, nil, s.repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	recovered, err := admin.ClearAccountError(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Nil(recovered.TempUnschedulableUntil)
	generation, ok := recovered.Extra["first_output_recovery_generation"].(string)
	s.Require().True(ok)
	s.Require().NotEmpty(generation)
	applied, err = s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, "", until, reason)
	s.Require().NoError(err)
	s.Require().False(applied, "in-flight request must not undo recovery")
	// A stale ordinary edit or extra update cannot restore the old generation.
	s.Require().NoError(s.repo.Update(s.ctx, account))
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"first_output_recovery_generation": "stale", "note": "updated"}))
	current, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(generation, current.Extra["first_output_recovery_generation"])
	applied, err = s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, generation, until, reason)
	s.Require().NoError(err)
	s.Require().True(applied, "new requests can trip normally")
	// A reason-specific background recovery cannot erase a newly installed pause.
	cleared, err = s.repo.ClearTempUnschedulableIfReasonUnchanged(s.ctx, account.ID, "previous auth failure")
	s.Require().NoError(err)
	s.Require().False(cleared)
}

func TestFirstOutputLegacyManualRecoveryStartsNewRedisWindow(t *testing.T) {
	ctx := t.Context()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{Name: "first-output-window", Credentials: map[string]any{"temp_unschedulable_enabled": true, "temp_unschedulable_first_output_enabled": true}})
	_, cache := firstOutputTestCache(t)
	svc := service.NewRateLimitService(repo, nil, nil, nil, cache)
	svc.RecordFirstOutputLatency(ctx, account, "model", 31*time.Second)
	svc.RecordFirstOutputLatency(ctx, account, "model", 31*time.Second)
	admin := service.NewAdminService(nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	fresh, err := admin.ClearAccountError(ctx, account.ID)
	require.NoError(t, err)
	// The stale third sample does not stop the account.
	svc.RecordFirstOutputLatency(ctx, account, "model", 31*time.Second)
	svc.RecordFirstOutputLatency(ctx, fresh, "model", 31*time.Second)
	current, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, current.TempUnschedulableUntil)
	svc.RecordFirstOutputLatency(ctx, fresh, "model", 31*time.Second)
	svc.RecordFirstOutputLatency(ctx, fresh, "model", 31*time.Second)
	current, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, current.TempUnschedulableUntil)
}
