package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// checkinRepository 用裸 SQL 实现签到记录的读写。
//
// 与 affiliate 模块同构：user_checkin_records 只有一张扁平表，没有 ent schema，
// 通过已启用的 sql/execquery feature 在 ent 事务内执行原生 SQL，这样签到记录的
// 写入可以和余额变更共处一个事务。
type checkinRepository struct {
	client *dbent.Client
}

// NewCheckinRepository 构造签到仓储。
func NewCheckinRepository(client *dbent.Client) service.CheckinRepository {
	return &checkinRepository{client: client}
}

// uniqueViolationCode 是 PostgreSQL 唯一约束冲突的 SQLSTATE。
const uniqueViolationCode = "23505"

// Insert 写入一条签到记录。
//
// 命中 (user_id, checkin_date) 唯一索引时返回 service.ErrCheckinAlreadyDone —
// 这是判重的唯一权威来源：并发的两次请求里，只有插入成功的那一次才继续加余额。
func (r *checkinRepository) Insert(ctx context.Context, userID int64, date time.Time, amount float64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.ExecContext(ctx,
		`INSERT INTO user_checkin_records (user_id, checkin_date, amount, created_at)
		 VALUES ($1, $2, $3, NOW())`,
		userID, date.Format("2006-01-02"), amount,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == uniqueViolationCode {
			return service.ErrCheckinAlreadyDone
		}
		return fmt.Errorf("insert checkin record: %w", err)
	}
	return nil
}

// CreditBalance 给用户加余额。
//
// 刻意不复用 userRepo.UpdateBalance：那条路径在 amount > 0 时会同时累加
// users.total_recharged，而 total_recharged 是「累计充值」，用于按百分比计算
// 低余额提醒阈值（balance_notify_service.resolveBalanceThreshold）。签到发的是
// 奖励不是充值，计进去会让阈值随签到天数漂移，也会让从未付费的用户显示有充值额。
func (r *checkinRepository) CreditBalance(ctx context.Context, userID int64, amount float64) error {
	client := clientFromContext(ctx, r.client)
	res, err := client.ExecContext(ctx,
		`UPDATE users SET balance = balance + $1, updated_at = NOW()
		  WHERE id = $2 AND deleted_at IS NULL`,
		amount, userID,
	)
	if err != nil {
		return fmt.Errorf("credit checkin balance: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("credit checkin balance rows: %w", err)
	}
	if affected == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

// ListByMonth 返回某个自然月内该用户的全部签到记录，按日期升序。
func (r *checkinRepository) ListByMonth(ctx context.Context, userID int64, monthStart, monthEnd time.Time) ([]service.CheckinRecord, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT checkin_date, amount
		   FROM user_checkin_records
		  WHERE user_id = $1 AND checkin_date >= $2 AND checkin_date < $3
		  ORDER BY checkin_date ASC`,
		userID, monthStart.Format("2006-01-02"), monthEnd.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("list checkin records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := make([]service.CheckinRecord, 0, 31)
	for rows.Next() {
		var (
			date   time.Time
			amount float64
		)
		if err := rows.Scan(&date, &amount); err != nil {
			return nil, fmt.Errorf("scan checkin record: %w", err)
		}
		records = append(records, service.CheckinRecord{Date: date, Amount: amount})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkin records: %w", err)
	}
	return records, nil
}

// CountAndSum 返回该用户的累计签到次数与累计获得金额。
func (r *checkinRepository) CountAndSum(ctx context.Context, userID int64) (int, float64, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(amount), 0) FROM user_checkin_records WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("aggregate checkin records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var (
		count int
		total float64
	)
	if rows.Next() {
		if err := rows.Scan(&count, &total); err != nil {
			return 0, 0, fmt.Errorf("scan checkin aggregate: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("iterate checkin aggregate: %w", err)
	}
	return count, total, nil
}

// WithTx 在事务中执行 fn。已经处于事务中时直接复用外层事务。
func (r *checkinRepository) WithTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin checkin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(dbent.NewTxContext(ctx, tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit checkin transaction: %w", err)
	}
	return nil
}
