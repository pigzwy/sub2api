//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeCheckinRepo 是内存版签到仓储，用 (userID, date) 组合键复刻数据库那条唯一索引。
type fakeCheckinRepo struct {
	records   map[int64]map[string]float64
	credited  map[int64]float64
	insertErr error
	creditErr error
}

func newFakeCheckinRepo() *fakeCheckinRepo {
	return &fakeCheckinRepo{records: map[int64]map[string]float64{}, credited: map[int64]float64{}}
}

func (f *fakeCheckinRepo) Insert(_ context.Context, userID int64, date time.Time, amount float64) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	key := date.Format("2006-01-02")
	if _, ok := f.records[userID][key]; ok {
		return ErrCheckinAlreadyDone
	}
	if f.records[userID] == nil {
		f.records[userID] = map[string]float64{}
	}
	f.records[userID][key] = amount
	return nil
}

func (f *fakeCheckinRepo) CreditBalance(_ context.Context, userID int64, amount float64) error {
	if f.creditErr != nil {
		return f.creditErr
	}
	f.credited[userID] += amount
	return nil
}

func (f *fakeCheckinRepo) ListByMonth(_ context.Context, userID int64, monthStart, monthEnd time.Time) ([]CheckinRecord, error) {
	var out []CheckinRecord
	for key, amount := range f.records[userID] {
		day, err := time.ParseInLocation("2006-01-02", key, monthStart.Location())
		if err != nil {
			continue
		}
		if !day.Before(monthStart) && day.Before(monthEnd) {
			out = append(out, CheckinRecord{Date: day, Amount: amount})
		}
	}
	return out, nil
}

func (f *fakeCheckinRepo) CountAndSum(_ context.Context, userID int64) (int, float64, error) {
	var total float64
	for _, amount := range f.records[userID] {
		total += amount
	}
	return len(f.records[userID]), total, nil
}

func (f *fakeCheckinRepo) WithTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestRandomAmount_StaysWithinRange(t *testing.T) {
	// 抽样足够多次，确保取值不会越出配置区间——奖励直接变成余额，越界即是发错钱。
	for i := 0; i < 500; i++ {
		got := randomAmount(0.1, 0.3)
		require.GreaterOrEqual(t, got, 0.1)
		require.LessOrEqual(t, got, 0.3)
	}
}

func TestRandomAmount_RoundsToTwoDecimals(t *testing.T) {
	for i := 0; i < 200; i++ {
		got := randomAmount(1.005, 9.995)
		require.InDelta(t, got, roundCheckinAmount(got), 1e-9, "amount must already be 2-decimal rounded")
	}
}

func TestRandomAmount_DegenerateRange(t *testing.T) {
	require.Equal(t, 0.5, randomAmount(0.5, 0.5))
	// 上限低于下限属于脏配置：按下限发，不放大支出。
	require.Equal(t, 0.5, randomAmount(0.5, 0.2))
}

func TestRandomAmount_CanReachBothEnds(t *testing.T) {
	// 区间只有两个可能取值时，两端都应该出现，说明取样没有系统性偏移。
	seen := map[float64]bool{}
	for i := 0; i < 400; i++ {
		seen[randomAmount(0.10, 0.11)] = true
	}
	require.True(t, seen[0.10], "lower bound should be reachable")
	require.True(t, seen[0.11], "upper bound should be reachable")
}

func TestCheckinAlreadyDone_IsDetectable(t *testing.T) {
	repo := newFakeCheckinRepo()
	today := time.Now()

	require.NoError(t, repo.Insert(context.Background(), 1, today, 0.2))

	err := repo.Insert(context.Background(), 1, today, 0.2)
	require.Error(t, err)
	require.True(t, IsCheckinAlreadyDone(err))
	require.True(t, errors.Is(err, ErrCheckinAlreadyDone))

	// 判重是按 (用户, 日期) 的：换个用户当天仍可签。
	require.NoError(t, repo.Insert(context.Background(), 2, today, 0.2))
	// 同一用户换一天也可以。
	require.NoError(t, repo.Insert(context.Background(), 1, today.AddDate(0, 0, 1), 0.2))
}
