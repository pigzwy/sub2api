//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type affiliateSettingRepoStub struct {
	values map[string]string
	err    error
}

func (s *affiliateSettingRepoStub) Get(context.Context, string) (*Setting, error) { return nil, s.err }
func (s *affiliateSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.values != nil {
		if value, ok := s.values[key]; ok {
			return value, nil
		}
	}
	return "", ErrSettingNotFound
}
func (s *affiliateSettingRepoStub) Set(context.Context, string, string) error { return s.err }
func (s *affiliateSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return map[string]string{}, nil
}
func (s *affiliateSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return s.err
}
func (s *affiliateSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return map[string]string{}, nil
}
func (s *affiliateSettingRepoStub) Delete(context.Context, string) error { return s.err }

type affiliateRepoStub struct {
	summaries map[int64]*AffiliateSummary

	firstRechargeApplied bool
	firstRechargeAmount  float64
	firstRechargeFreeze  int
	firstRechargeSource  string
	firstRechargeInviter int64
	firstRechargeInvitee int64
}

func (s *affiliateRepoStub) EnsureUserAffiliate(_ context.Context, userID int64) (*AffiliateSummary, error) {
	if s.summaries == nil {
		s.summaries = map[int64]*AffiliateSummary{}
	}
	if summary, ok := s.summaries[userID]; ok {
		return summary, nil
	}
	summary := &AffiliateSummary{UserID: userID, AffCode: "ABCDEFGHJKLM", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.summaries[userID] = summary
	return summary, nil
}

func (s *affiliateRepoStub) GetAffiliateByCode(context.Context, string) (*AffiliateSummary, error) {
	return nil, ErrAffiliateProfileNotFound
}

func (s *affiliateRepoStub) BindInviter(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func (s *affiliateRepoStub) AccrueQuota(context.Context, int64, int64, float64, int) (bool, error) {
	return false, nil
}

func (s *affiliateRepoStub) AccrueFirstRechargeQuota(_ context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceRef string) (bool, error) {
	s.firstRechargeAmount = amount
	s.firstRechargeFreeze = freezeHours
	s.firstRechargeSource = sourceRef
	s.firstRechargeInviter = inviterID
	s.firstRechargeInvitee = inviteeUserID
	return s.firstRechargeApplied, nil
}

func (s *affiliateRepoStub) GetAccruedRebateFromInvitee(context.Context, int64, int64) (float64, error) {
	return 0, nil
}

func (s *affiliateRepoStub) ThawFrozenQuota(context.Context, int64) (float64, error) {
	return 0, nil
}

func (s *affiliateRepoStub) TransferQuotaToBalance(context.Context, int64) (float64, float64, error) {
	return 0, 0, nil
}

func (s *affiliateRepoStub) ListInvitees(context.Context, int64, int) ([]AffiliateInvitee, error) {
	return nil, nil
}

func (s *affiliateRepoStub) UpdateUserAffCode(context.Context, int64, string) error {
	return nil
}

func (s *affiliateRepoStub) ResetUserAffCode(context.Context, int64) (string, error) {
	return "", nil
}

func (s *affiliateRepoStub) SetUserRebateRate(context.Context, int64, *float64) error {
	return nil
}

func (s *affiliateRepoStub) BatchSetUserRebateRate(context.Context, []int64, *float64) error {
	return nil
}

func (s *affiliateRepoStub) ListUsersWithCustomSettings(context.Context, AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	return nil, 0, nil
}

func affiliateTestSettingService(rate string) *SettingService {
	return &SettingService{settingRepo: &affiliateSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateEnabled:             "true",
		SettingKeyAffiliateRebateRate:          rate,
		SettingKeyAffiliateRebateFreezeHours:   "0",
		SettingKeyAffiliateRebateDurationDays:  "0",
		SettingKeyAffiliateRebatePerInviteeCap: "0",
	}}}
}

// TestResolveRebateRatePercent_PerUserOverride verifies that per-inviter
// AffRebateRatePercent overrides the global rate, that NULL falls back to the
// global rate, and that out-of-range exclusive rates are clamped silently.
//
// SettingService is left nil here so globalRebateRatePercent returns the
// documented default (AffiliateRebateRateDefault = 20%) — this exercises the
// fallback path without spinning up a settings stub.
func TestResolveRebateRatePercent_PerUserOverride(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}

	// nil exclusive rate → falls back to global default (20%)
	require.InDelta(t, AffiliateRebateRateDefault,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{}), 1e-9)

	// exclusive rate set → overrides global
	rate := 50.0
	require.InDelta(t, 50.0,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &rate}), 1e-9)

	// exclusive rate 0 → returns 0 (no rebate, intentional)
	zero := 0.0
	require.InDelta(t, 0.0,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &zero}), 1e-9)

	// exclusive rate above max → clamped to Max
	tooHigh := 250.0
	require.InDelta(t, AffiliateRebateRateMax,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &tooHigh}), 1e-9)

	// exclusive rate below min → clamped to Min
	tooLow := -5.0
	require.InDelta(t, AffiliateRebateRateMin,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &tooLow}), 1e-9)
}

// TestIsEnabled_NilSettingServiceReturnsDefault verifies that IsEnabled
// safely handles a nil settingService dependency by returning the fork default.
// This protects callers from nil-pointer crashes in misconfigured environments.
func TestIsEnabled_NilSettingServiceReturnsDefault(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}
	require.Equal(t, AffiliateEnabledDefault, svc.IsEnabled(context.Background()))
}

// TestValidateExclusiveRate_BoundaryAndInvalid covers the validator used by
// admin-facing rate setters: nil is always valid (clear), in-range values
// are accepted, NaN/Inf and out-of-range values produce a typed BadRequest.
func TestValidateExclusiveRate_BoundaryAndInvalid(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateExclusiveRate(nil))

	for _, v := range []float64{0, 0.01, 50, 99.99, 100} {
		v := v
		require.NoError(t, validateExclusiveRate(&v), "value %v should be valid", v)
	}

	for _, v := range []float64{-0.01, 100.01, -100, 200} {
		v := v
		require.Error(t, validateExclusiveRate(&v), "value %v should be rejected", v)
	}

	nan := math.NaN()
	require.Error(t, validateExclusiveRate(&nan))
	posInf := math.Inf(1)
	require.Error(t, validateExclusiveRate(&posInf))
	negInf := math.Inf(-1)
	require.Error(t, validateExclusiveRate(&negInf))
}

func TestAccrueFirstRechargeRebate_UsesFirstRechargeClaim(t *testing.T) {
	t.Parallel()

	inviterID := int64(10)
	inviteeID := int64(20)
	repo := &affiliateRepoStub{
		firstRechargeApplied: true,
		summaries: map[int64]*AffiliateSummary{
			inviteeID: {UserID: inviteeID, InviterID: &inviterID},
			inviterID: {UserID: inviterID},
		},
	}
	svc := &AffiliateService{
		repo:           repo,
		settingService: affiliateTestSettingService("10"),
	}

	rebate, err := svc.AccrueFirstRechargeRebate(context.Background(), inviteeID, 80, "redeem:s2p_order")
	require.NoError(t, err)
	require.Equal(t, 8.0, rebate)
	require.Equal(t, 8.0, repo.firstRechargeAmount)
	require.Equal(t, 0, repo.firstRechargeFreeze)
	require.Equal(t, "redeem:s2p_order", repo.firstRechargeSource)
	require.Equal(t, inviterID, repo.firstRechargeInviter)
	require.Equal(t, inviteeID, repo.firstRechargeInvitee)
}

func TestAccrueFirstRechargeRebate_ReturnsZeroWhenAlreadyClaimed(t *testing.T) {
	t.Parallel()

	inviterID := int64(10)
	inviteeID := int64(20)
	repo := &affiliateRepoStub{
		firstRechargeApplied: false,
		summaries: map[int64]*AffiliateSummary{
			inviteeID: {UserID: inviteeID, InviterID: &inviterID},
			inviterID: {UserID: inviterID},
		},
	}
	svc := &AffiliateService{
		repo:           repo,
		settingService: affiliateTestSettingService("10"),
	}

	rebate, err := svc.AccrueFirstRechargeRebate(context.Background(), inviteeID, 80, "redeem:s2p_order")
	require.NoError(t, err)
	require.Equal(t, 0.0, rebate)
}

func TestMaskEmail(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a***@g***.com", maskEmail("alice@gmail.com"))
	require.Equal(t, "x***@d***", maskEmail("x@domain"))
	require.Equal(t, "", maskEmail(""))
}

func TestIsValidAffiliateCodeFormat(t *testing.T) {
	t.Parallel()

	// 邀请码格式校验同时服务于：
	// 1) 系统自动生成的 12 位随机码（A-Z 去 I/O，2-9 去 0/1）
	// 2) 管理员设置的自定义专属码（如 "VIP2026"、"NEW_USER-1"）
	// 因此校验放宽到 [A-Z0-9_-]{4,32}（要求调用方先 ToUpper）。
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid canonical 12-char", "ABCDEFGHJKLM", true},
		{"valid all digits 2-9", "234567892345", true},
		{"valid mixed", "A2B3C4D5E6F7", true},
		{"valid admin custom short", "VIP1", true},
		{"valid admin custom with hyphen", "NEW-USER", true},
		{"valid admin custom with underscore", "VIP_2026", true},
		{"valid 32-char max", "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345", true},
		// Previously-excluded chars (I/O/0/1) are now allowed since admins may use them.
		{"letter I now allowed", "IBCDEFGHJKLM", true},
		{"letter O now allowed", "OBCDEFGHJKLM", true},
		{"digit 0 now allowed", "0BCDEFGHJKLM", true},
		{"digit 1 now allowed", "1BCDEFGHJKLM", true},
		{"too short (3 chars)", "ABC", false},
		{"too long (33 chars)", "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456", false},
		{"lowercase rejected (caller must ToUpper first)", "abcdefghjklm", false},
		{"empty", "", false},
		{"utf8 non-ascii", "ÄÄÄÄÄÄ", false}, // bytes out of charset
		{"ascii punctuation .", "ABCDEFGHJK.M", false},
		{"whitespace", "ABCDEFGHJK M", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isValidAffiliateCodeFormat(tc.in))
		})
	}
}
