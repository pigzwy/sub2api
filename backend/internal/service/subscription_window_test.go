package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type windowMaintenanceUserSubRepoStub struct {
	userSubRepoNoop

	activatedDailyAt    *time.Time
	activatedPeriodicAt *time.Time
	resetDailyAt        *time.Time
	resetWeeklyAt       *time.Time
	resetMonthlyAt      *time.Time
}

func (r *windowMaintenanceUserSubRepoStub) ActivateWindows(_ context.Context, _ int64, dailyStart, periodicStart time.Time) error {
	r.activatedDailyAt = &dailyStart
	r.activatedPeriodicAt = &periodicStart
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
	now := time.Date(2026, 8, 7, 14, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	sub := &UserSubscription{ID: 1}

	err := svc.CheckAndActivateWindow(context.Background(), sub)

	require.NoError(t, err)
	require.NotNil(t, repo.activatedDailyAt)
	require.NotNil(t, repo.activatedPeriodicAt)
	require.Equal(t, timezone.StartOfDay(now), *repo.activatedDailyAt)
	require.Equal(t, now, *repo.activatedPeriodicAt)
}

func TestCheckAndResetWindows_ResetsExpiredDailyWindowAtMidnight(t *testing.T) {
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
	require.Equal(t, timezone.StartOfDay(now), *repo.resetDailyAt)
	require.Equal(t, float64(0), sub.DailyUsageUSD)
	require.Equal(t, repo.resetDailyAt, sub.DailyWindowStart)
}
