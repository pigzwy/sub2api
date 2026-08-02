package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type windowMaintenanceUserSubRepoStub struct {
	userSubRepoNoop

	activatedAt    *time.Time
	resetDailyAt   *time.Time
	resetWeeklyAt  *time.Time
	resetMonthlyAt *time.Time
}

func (r *windowMaintenanceUserSubRepoStub) ActivateWindows(_ context.Context, _ int64, start time.Time) error {
	r.activatedAt = &start
	return nil
}

func (r *windowMaintenanceUserSubRepoStub) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, start time.Time) error {
	r.resetDailyAt = &start
	return nil
}

func (r *windowMaintenanceUserSubRepoStub) ResetWeeklyUsage(_ context.Context, _ int64, _ *time.Time, start time.Time) error {
	r.resetWeeklyAt = &start
	return nil
}

func (r *windowMaintenanceUserSubRepoStub) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, start time.Time) error {
	r.resetMonthlyAt = &start
	return nil
}

func TestCheckAndActivateWindow_UsesCurrentTime(t *testing.T) {
	repo := &windowMaintenanceUserSubRepoStub{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{ID: 1}

	before := time.Now()
	err := svc.CheckAndActivateWindow(context.Background(), sub)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, repo.activatedAt)
	require.False(t, repo.activatedAt.Before(before), "窗口不应回退到当天零点")
	require.False(t, repo.activatedAt.After(after), "窗口应使用当前激活时间")
}

func TestCheckAndResetWindows_AdvancesExpiredDailyWindowFromSubscriptionAnchor(t *testing.T) {
	repo := &windowMaintenanceUserSubRepoStub{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	oldWindow := now.Add(-25 * time.Hour)
	svc.now = func() time.Time { return now }
	sub := &UserSubscription{
		ID:               1,
		StartsAt:         oldWindow,
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		DailyWindowStart: &oldWindow,
		DailyUsageUSD:    10,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.NotNil(t, repo.resetDailyAt)
	require.Equal(t, oldWindow.Add(24*time.Hour), *repo.resetDailyAt)
	require.Equal(t, float64(0), sub.DailyUsageUSD)
	require.Equal(t, repo.resetDailyAt, sub.DailyWindowStart)
}
