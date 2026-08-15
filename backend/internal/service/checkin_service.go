package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"go.uber.org/zap"
)

// 签到相关的业务错误。
var (
	// ErrCheckinDisabled 表示签到功能未开启。
	ErrCheckinDisabled = infraerrors.NotFound("CHECKIN_DISABLED", "check-in is not enabled")
	// ErrCheckinAlreadyDone 表示今天已经签过了。
	ErrCheckinAlreadyDone = infraerrors.Conflict("CHECKIN_ALREADY_DONE", "already checked in today")
)

// CheckinRecord 是一条签到记录。
type CheckinRecord struct {
	Date   time.Time
	Amount float64
}

// CheckinRepository 是签到记录的持久化接口。
type CheckinRepository interface {
	// Insert 写入一条签到记录；唯一约束冲突时返回 ErrCheckinAlreadyDone。
	Insert(ctx context.Context, userID int64, date time.Time, amount float64) error
	// CreditBalance 给用户加余额（只动 balance，不计入累计充值）。
	CreditBalance(ctx context.Context, userID int64, amount float64) error
	// RecordBalanceHistory 写一条余额变动记录，使签到出现在余额记录页。
	RecordBalanceHistory(ctx context.Context, userID int64, code string, amount float64, notes string) error
	// ListByMonth 返回 [monthStart, monthEnd) 区间内的记录，按日期升序。
	ListByMonth(ctx context.Context, userID int64, monthStart, monthEnd time.Time) ([]CheckinRecord, error)
	// CountAndSum 返回累计签到次数与累计金额。
	CountAndSum(ctx context.Context, userID int64) (int, float64, error)
	// WithTx 在事务中执行 fn。
	WithTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

// CheckinSnapshot 是签到页需要的全部数据。
type CheckinSnapshot struct {
	Enabled         bool    `json:"enabled"`
	CaptchaEnabled  bool    `json:"captcha_enabled"`
	Today           string  `json:"today"`
	YearMonth       string  `json:"year_month"`
	SignedToday     bool    `json:"signed_today"`
	MonthSignedDays int     `json:"month_signed_days"`
	TotalDays       int     `json:"total_days"`
	TotalAmount     float64 `json:"total_amount"`
	Balance         float64 `json:"balance"`
}

// CheckinResult 是一次成功签到的结果。
type CheckinResult struct {
	Amount   float64          `json:"amount"`
	Date     string           `json:"date"`
	Snapshot *CheckinSnapshot `json:"snapshot"`
}

// CheckinService 实现每日签到：每天一次，发放区间内的随机余额奖励。
type CheckinService struct {
	repo           CheckinRepository
	userRepo       UserRepository
	settingService *SettingService
	billingCache   *BillingCacheService
}

// NewCheckinService 构造签到服务。
func NewCheckinService(
	repo CheckinRepository,
	userRepo UserRepository,
	settingService *SettingService,
	billingCache *BillingCacheService,
) *CheckinService {
	return &CheckinService{
		repo:           repo,
		userRepo:       userRepo,
		settingService: settingService,
		billingCache:   billingCache,
	}
}

// IsEnabled 报告签到功能是否开启。
func (s *CheckinService) IsEnabled(ctx context.Context) bool {
	if s == nil || s.settingService == nil {
		return false
	}
	return s.settingService.GetCheckinConfig(ctx).Enabled
}

// config 读取运行时配置。
func (s *CheckinService) config(ctx context.Context) CheckinConfig {
	if s == nil || s.settingService == nil {
		return CheckinConfig{}
	}
	return s.settingService.GetCheckinConfig(ctx)
}

// GetSnapshot 返回当月签到日历与统计。功能关闭时返回 ErrCheckinDisabled。
func (s *CheckinService) GetSnapshot(ctx context.Context, userID int64) (*CheckinSnapshot, error) {
	cfg := s.config(ctx)
	if !cfg.Enabled {
		return nil, ErrCheckinDisabled
	}
	return s.buildSnapshot(ctx, userID, cfg)
}

// Checkin 执行一次签到。
//
// 记录写入与余额发放在同一事务里完成：任何一步失败都整体回滚，于是「今天已签到」
// 与「余额已到账」永远一致——入账失败时这一天不会被标记为已签，用户可以重试。
func (s *CheckinService) Checkin(ctx context.Context, userID int64) (*CheckinResult, error) {
	cfg := s.config(ctx)
	if !cfg.Enabled {
		return nil, ErrCheckinDisabled
	}

	today := s.today()
	amount := randomAmount(cfg.MinAmount, cfg.MaxAmount)

	historyCode, err := GenerateRedeemCode()
	if err != nil {
		return nil, fmt.Errorf("generate checkin history code: %w", err)
	}

	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		// 先插签到记录：唯一约束是并发下唯一可靠的判重点，插入失败就不会走到加余额。
		if err := s.repo.Insert(txCtx, userID, today, amount); err != nil {
			return err
		}
		if err := s.repo.CreditBalance(txCtx, userID, amount); err != nil {
			return err
		}
		// 余额变动记录与前两步同事务：三者要么都成立，要么都不成立，
		// 不会出现「加了钱查不到记录」或「有记录但没加钱」。
		return s.repo.RecordBalanceHistory(txCtx, userID, historyCode, amount, checkinHistoryNotes(today))
	})
	if err != nil {
		return nil, err
	}

	// 事务提交后再失效缓存，避免回滚时把陈旧值重新灌回缓存。
	// 失效失败只记日志不返回错误：钱已经到账，缓存至多让余额显示滞后一小会儿，
	// 不该让用户看到「签到失败」而重试（重试也会因唯一约束被判为今日已签）。
	if s.billingCache != nil {
		if cacheErr := s.billingCache.InvalidateUserBalance(ctx, userID); cacheErr != nil {
			logger.L().Warn("checkin.invalidate_balance_cache_failed",
				zap.Int64("user_id", userID), zap.Error(cacheErr))
		}
	}

	snapshot, err := s.buildSnapshot(ctx, userID, cfg)
	if err != nil {
		// 奖励已经发放成功，快照只是展示数据，构建失败不该让调用方以为签到失败。
		snapshot = nil
	}
	return &CheckinResult{
		Amount:   amount,
		Date:     today.Format("2006-01-02"),
		Snapshot: snapshot,
	}, nil
}

// buildSnapshot 组装当月日历与累计统计。
func (s *CheckinService) buildSnapshot(ctx context.Context, userID int64, cfg CheckinConfig) (*CheckinSnapshot, error) {
	now := timezone.Now()
	today := s.today()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)

	records, err := s.repo.ListByMonth(ctx, userID, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	byDay := make(map[int]float64, len(records))
	for _, record := range records {
		byDay[record.Date.Day()] = record.Amount
	}

	totalDays, totalAmount, err := s.repo.CountAndSum(ctx, userID)
	if err != nil {
		return nil, err
	}

	snapshot := &CheckinSnapshot{
		Enabled:         true,
		CaptchaEnabled:  cfg.CaptchaEnabled,
		Today:           today.Format("2006-01-02"),
		YearMonth:       now.Format("2006-01"),
		MonthSignedDays: len(records),
		TotalDays:       totalDays,
		TotalAmount:     roundCheckinAmount(totalAmount),
	}
	if _, ok := byDay[today.Day()]; ok {
		snapshot.SignedToday = true
	}

	// 余额只是展示字段：仓储缺失或读取失败时留 0，不要让整次签到因此报错——
	// 此时奖励可能已经入账，快照失败会让调用方误以为签到没成功。
	if s.userRepo != nil {
		if user, err := s.userRepo.GetByID(ctx, userID); err == nil && user != nil {
			snapshot.Balance = user.Balance
		}
	}
	return snapshot, nil
}

// today 返回服务器时区下的当天零点，与 checkin_date 的语义一致。
func (s *CheckinService) today() time.Time {
	now := timezone.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// randomAmount 在 [min, max] 内取一个两位小数的随机金额。
//
// 用 crypto/rand 而不是 math/rand：奖励金额直接变成余额，可预测的序列会让人能
// 挑时机签到。区间退化（min == max）时直接返回该值。
func randomAmount(min, max float64) float64 {
	if max <= min {
		return roundCheckinAmount(min)
	}
	// 以「分」为单位取整数随机数，避免浮点区间取样后再四舍五入越界。
	span := int64(math.Round((max - min) * 100))
	if span <= 0 {
		return roundCheckinAmount(min)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(span+1))
	if err != nil {
		// 熵不可用时退回下限：宁可少发，不可多发。
		return roundCheckinAmount(min)
	}
	return roundCheckinAmount(min + float64(n.Int64())/100)
}

// roundCheckinAmount 统一保留两位小数。
func roundCheckinAmount(v float64) float64 {
	return math.Round(v*100) / 100
}

// IsCheckinAlreadyDone 报告 err 是否为「今天已签到」。
func IsCheckinAlreadyDone(err error) bool {
	return errors.Is(err, ErrCheckinAlreadyDone)
}

// CaptchaRequired 报告签到是否要求人机验证。
func (s *CheckinService) CaptchaRequired(ctx context.Context) bool {
	cfg := s.config(ctx)
	return cfg.Enabled && cfg.CaptchaEnabled
}

// checkinHistoryNotes 生成余额记录里的备注，带上签到日期便于对账。
func checkinHistoryNotes(day time.Time) string {
	return "daily check-in " + day.Format("2006-01-02")
}
