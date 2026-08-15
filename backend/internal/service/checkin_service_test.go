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
	records    map[int64]map[string]float64
	credited   map[int64]float64
	history    map[int64][]float64
	insertErr  error
	creditErr  error
	historyErr error
}

func newFakeCheckinRepo() *fakeCheckinRepo {
	return &fakeCheckinRepo{
		records:  map[int64]map[string]float64{},
		credited: map[int64]float64{},
		history:  map[int64][]float64{},
	}
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

func (f *fakeCheckinRepo) RecordBalanceHistory(_ context.Context, userID int64, _ string, amount float64, _ string) error {
	if f.historyErr != nil {
		return f.historyErr
	}
	f.history[userID] = append(f.history[userID], amount)
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

// txFakeCheckinRepo 在 fakeCheckinRepo 之上模拟真实事务：fn 返回错误时丢弃本次
// 事务内的全部写入。用来验证「签到记录 / 加余额 / 余额流水」三者同生共死。
type txFakeCheckinRepo struct {
	*fakeCheckinRepo
}

func (f *txFakeCheckinRepo) WithTx(ctx context.Context, fn func(context.Context) error) error {
	before := snapshotRepo(f.fakeCheckinRepo)
	if err := fn(ctx); err != nil {
		restoreRepo(f.fakeCheckinRepo, before)
		return err
	}
	return nil
}

type repoSnapshot struct {
	records  map[int64]map[string]float64
	credited map[int64]float64
	history  map[int64][]float64
}

func snapshotRepo(f *fakeCheckinRepo) repoSnapshot {
	s := repoSnapshot{
		records:  map[int64]map[string]float64{},
		credited: map[int64]float64{},
		history:  map[int64][]float64{},
	}
	for u, days := range f.records {
		s.records[u] = map[string]float64{}
		for d, a := range days {
			s.records[u][d] = a
		}
	}
	for u, a := range f.credited {
		s.credited[u] = a
	}
	for u, hs := range f.history {
		s.history[u] = append([]float64(nil), hs...)
	}
	return s
}

func restoreRepo(f *fakeCheckinRepo, s repoSnapshot) {
	f.records, f.credited, f.history = s.records, s.credited, s.history
}

// 余额流水写入失败时，签到记录与余额都必须一并回滚——否则会出现「加了钱查不到
// 记录」，正是这次补流水想要消灭的状态。
func TestCheckin_HistoryFailureRollsBackEverything(t *testing.T) {
	fake := newFakeCheckinRepo()
	fake.historyErr = errors.New("history write failed")
	repo := &txFakeCheckinRepo{fakeCheckinRepo: fake}

	err := repo.WithTx(context.Background(), func(ctx context.Context) error {
		if err := repo.Insert(ctx, 7, time.Now(), 0.2); err != nil {
			return err
		}
		if err := repo.CreditBalance(ctx, 7, 0.2); err != nil {
			return err
		}
		return repo.RecordBalanceHistory(ctx, 7, "code", 0.2, "notes")
	})

	require.Error(t, err)
	require.Empty(t, fake.records[7], "签到记录必须回滚，否则用户当天再也签不了")
	require.Zero(t, fake.credited[7], "余额必须回滚")
	require.Empty(t, fake.history[7])
}

// 成功路径：三者都落地，且金额一致。
func TestCheckin_SuccessWritesAllThree(t *testing.T) {
	fake := newFakeCheckinRepo()
	repo := &txFakeCheckinRepo{fakeCheckinRepo: fake}
	today := time.Now()

	err := repo.WithTx(context.Background(), func(ctx context.Context) error {
		if err := repo.Insert(ctx, 7, today, 0.25); err != nil {
			return err
		}
		if err := repo.CreditBalance(ctx, 7, 0.25); err != nil {
			return err
		}
		return repo.RecordBalanceHistory(ctx, 7, "code", 0.25, "notes")
	})

	require.NoError(t, err)
	require.Equal(t, 0.25, fake.records[7][today.Format("2006-01-02")])
	require.Equal(t, 0.25, fake.credited[7])
	require.Equal(t, []float64{0.25}, fake.history[7])
}

func TestCheckinHistoryNotes_CarriesDate(t *testing.T) {
	day := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	require.Equal(t, "daily check-in 2026-08-15", checkinHistoryNotes(day))
}
