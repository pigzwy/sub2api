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

func (r *windowMaintenanceUserSubRepoStub) ResetDailyUsage(_ context.Context, _ int64, start time.Time) error {
	r.resetDailyAt = &start
	return nil
}

func (r *windowMaintenanceUserSubRepoStub) ResetWeeklyUsage(_ context.Context, _ int64, start time.Time) error {
	r.resetWeeklyAt = &start
	return nil
}

func (r *windowMaintenanceUserSubRepoStub) ResetMonthlyUsage(_ context.Context, _ int64, start time.Time) error {
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

func TestCheckAndResetWindows_UsesCurrentTimeForExpiredDailyWindow(t *testing.T) {
	repo := &windowMaintenanceUserSubRepoStub{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	oldWindow := time.Now().Add(-25 * time.Hour)
	sub := &UserSubscription{
		ID:               1,
		DailyWindowStart: &oldWindow,
		DailyUsageUSD:    10,
	}

	before := time.Now()
	err := svc.CheckAndResetWindows(context.Background(), sub)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, repo.resetDailyAt)
	require.False(t, repo.resetDailyAt.Before(before), "重置窗口不应回退到当天零点")
	require.False(t, repo.resetDailyAt.After(after), "重置窗口应使用当前重置时间")
	require.Equal(t, float64(0), sub.DailyUsageUSD)
	require.Equal(t, repo.resetDailyAt, sub.DailyWindowStart)
}
