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
	applied, err := s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, "", service.DefaultFirstOutputProtectionConfig(), until, reason)
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
	applied, err = s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, "", service.DefaultFirstOutputProtectionConfig(), until, reason)
	s.Require().NoError(err)
	s.Require().False(applied, "in-flight request must not undo recovery")
	// A stale ordinary edit or extra update cannot restore the old generation.
	s.Require().NoError(s.repo.Update(s.ctx, account))
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"first_output_recovery_generation": "stale", "note": "updated"}))
	current, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(generation, current.Extra["first_output_recovery_generation"])
	applied, err = s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, generation, service.DefaultFirstOutputProtectionConfig(), until, reason)
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

func (s *AccountRepoSuite) TestFirstOutputConfigCASAndLegacyDefaults() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "config-fence", Credentials: map[string]any{
		"temp_unschedulable_enabled": true, "temp_unschedulable_first_output_enabled": true,
	}})
	oldConfig := service.DefaultFirstOutputProtectionConfig()
	next := service.FirstOutputProtectionConfig{LimitSeconds: 45, CooldownMinutes: 12, WindowMinutes: 20}
	_, err := s.client.ExecContext(s.ctx, `UPDATE accounts SET credentials=credentials || $2::jsonb WHERE id=$1`, account.ID, `{"temp_unschedulable_first_output_limit_seconds":45,"temp_unschedulable_first_output_cooldown_minutes":12,"temp_unschedulable_first_output_window_minutes":20}`)
	s.Require().NoError(err)
	applied, err := s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, "", oldConfig, time.Now().Add(time.Minute), "old rule")
	s.Require().NoError(err)
	s.Require().False(applied)
	applied, err = s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, "", next, time.Now().Add(time.Minute), "current rule")
	s.Require().NoError(err)
	s.Require().True(applied)
	// Invalid legacy fields fall back safely, without PostgreSQL cast errors.
	for _, raw := range []string{`"bad"`, `null`, `{}`, `0`, `1.5`, `3601`, `true`, `30.0`} {
		_, err = s.client.ExecContext(s.ctx, `UPDATE accounts SET temp_unschedulable_until=NULL, credentials=jsonb_build_object('temp_unschedulable_enabled',true,'temp_unschedulable_first_output_enabled',true,'temp_unschedulable_first_output_limit_seconds',$2::jsonb) WHERE id=$1`, account.ID, raw)
		s.Require().NoError(err)
		applied, err = s.repo.SetTempUnschedulableIfUnchanged(s.ctx, account.ID, "", oldConfig, time.Now().Add(time.Minute), "defaults")
		s.Require().NoError(err)
		s.Require().True(applied, raw)
	}
}

func TestFirstOutputPolicyChangeAndManualRecoveryUseFreshCounters(t *testing.T) {
	ctx := t.Context()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	old := mustCreateAccount(t, tx.Client(), &service.Account{Name: "changing-policy", Credentials: map[string]any{
		"temp_unschedulable_enabled": true, "temp_unschedulable_first_output_enabled": true,
	}})
	_, cache := firstOutputTestCache(t)
	svc := service.NewRateLimitService(repo, nil, nil, nil, cache)
	for i := 0; i < 2; i++ {
		svc.RecordFirstOutputLatency(ctx, old, "model", 31*time.Second)
	}
	_, err := tx.Client().ExecContext(ctx, `UPDATE accounts SET credentials=credentials || $2::jsonb WHERE id=$1`, old.ID, `{"temp_unschedulable_first_output_limit_seconds":45,"temp_unschedulable_first_output_cooldown_minutes":12,"temp_unschedulable_first_output_window_minutes":20}`)
	require.NoError(t, err)
	current, err := repo.GetByID(ctx, old.ID)
	require.NoError(t, err)
	svc.RecordFirstOutputLatency(ctx, old, "model", 31*time.Second)
	svc.RecordFirstOutputLatency(ctx, current, "model", 46*time.Second)
	after, err := repo.GetByID(ctx, old.ID)
	require.NoError(t, err)
	require.Nil(t, after.TempUnschedulableUntil)
	for i := 0; i < 2; i++ {
		svc.RecordFirstOutputLatency(ctx, current, "model", 46*time.Second)
	}
	after, err = repo.GetByID(ctx, old.ID)
	require.NoError(t, err)
	require.NotNil(t, after.TempUnschedulableUntil)
	require.InDelta(t, 12*time.Minute, time.Until(*after.TempUnschedulableUntil), float64(time.Second))
	require.NoError(t, svc.ClearTempUnschedulable(ctx, old.ID))
	recovered, err := repo.GetByID(ctx, old.ID)
	require.NoError(t, err)
	svc.RecordFirstOutputLatency(ctx, recovered, "model", 46*time.Second)
	after, err = repo.GetByID(ctx, old.ID)
	require.NoError(t, err)
	require.Nil(t, after.TempUnschedulableUntil)
	// Completing a claim from before recovery cannot suppress the fresh policy.
	for i := 0; i < 2; i++ {
		svc.RecordFirstOutputLatency(ctx, recovered, "model", 46*time.Second)
	}
	after, err = repo.GetByID(ctx, old.ID)
	require.NoError(t, err)
	require.NotNil(t, after.TempUnschedulableUntil)
}

func (s *AccountRepoSuite) TestFirstOutputEditAndBulkPolicyRoundTripResetsWindow() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "policy-round-trip", Credentials: map[string]any{
		"temp_unschedulable_enabled": true, "temp_unschedulable_first_output_enabled": true,
	}})
	_, cache := firstOutputTestCache(s.T())
	svc := service.NewRateLimitService(s.repo, nil, nil, nil, cache)
	for i := 0; i < 2; i++ {
		svc.RecordFirstOutputLatency(s.ctx, account, "model", 31*time.Second)
	}
	changed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	changed.Credentials[service.FirstOutputLimitSecondsKey] = 45
	s.Require().NoError(s.repo.Update(s.ctx, changed))
	firstGeneration := changed.Extra["first_output_recovery_generation"]
	s.Require().NotEmpty(firstGeneration)
	// Returning to the original policy through bulk editing must not revive it.
	_, err = s.repo.BulkUpdate(s.ctx, []int64{account.ID}, service.AccountBulkUpdate{Credentials: map[string]any{service.FirstOutputLimitSecondsKey: 30}})
	s.Require().NoError(err)
	current, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotEqual(firstGeneration, current.Extra["first_output_recovery_generation"])
	svc.RecordFirstOutputLatency(s.ctx, account, "model", 31*time.Second)
	svc.RecordFirstOutputLatency(s.ctx, current, "model", 31*time.Second)
	after, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Nil(after.TempUnschedulableUntil)
	// Saving identical effective defaults or editing a name preserves this window.
	generation := current.Extra["first_output_recovery_generation"]
	current.Name = "renamed"
	current.Credentials[service.FirstOutputWindowMinutesKey] = 10
	current.Credentials[service.FirstOutputCooldownMinutesKey] = 5
	s.Require().NoError(s.repo.Update(s.ctx, current))
	s.Require().Equal(generation, current.Extra["first_output_recovery_generation"])
	_, err = s.repo.BulkUpdate(s.ctx, []int64{account.ID}, service.AccountBulkUpdate{Credentials: map[string]any{service.FirstOutputLimitSecondsKey: 30}})
	s.Require().NoError(err)
	after, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(generation, after.Extra["first_output_recovery_generation"])
	for i := 0; i < 2; i++ {
		svc.RecordFirstOutputLatency(s.ctx, after, "model", 31*time.Second)
	}
	after, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(after.TempUnschedulableUntil)
}
