//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type affiliateSettingRepoStub struct {
	value string
	err   error
}

func (s *affiliateSettingRepoStub) Get(context.Context, string) (*Setting, error) { return nil, s.err }
func (s *affiliateSettingRepoStub) GetValue(context.Context, string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.value, nil
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

func (s *affiliateRepoStub) AccrueQuota(context.Context, int64, int64, float64) (bool, error) {
	return false, nil
}

func (s *affiliateRepoStub) AccrueFirstRechargeQuota(_ context.Context, inviterID, inviteeUserID int64, amount float64, sourceRef string) (bool, error) {
	s.firstRechargeAmount = amount
	s.firstRechargeSource = sourceRef
	s.firstRechargeInviter = inviterID
	s.firstRechargeInvitee = inviteeUserID
	return s.firstRechargeApplied, nil
}

func (s *affiliateRepoStub) TransferQuotaToBalance(context.Context, int64) (float64, float64, error) {
	return 0, 0, nil
}

func (s *affiliateRepoStub) ListInvitees(context.Context, int64, int) ([]AffiliateInvitee, error) {
	return nil, nil
}

func TestAffiliateRebateRatePercentSemantics(t *testing.T) {
	t.Parallel()

	svc := &AffiliateService{settingRepo: &affiliateSettingRepoStub{value: "1"}}
	rate := svc.loadAffiliateRebateRatePercent(context.Background())
	require.Equal(t, 1.0, rate)

	svc.settingRepo = &affiliateSettingRepoStub{value: "0.2"}
	rate = svc.loadAffiliateRebateRatePercent(context.Background())
	require.Equal(t, 0.2, rate)
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
		repo:        repo,
		settingRepo: &affiliateSettingRepoStub{value: "10"},
	}

	rebate, err := svc.AccrueFirstRechargeRebate(context.Background(), inviteeID, 80, "redeem:s2p_order")
	require.NoError(t, err)
	require.Equal(t, 8.0, rebate)
	require.Equal(t, 8.0, repo.firstRechargeAmount)
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
		repo:        repo,
		settingRepo: &affiliateSettingRepoStub{value: "10"},
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

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid canonical", "ABCDEFGHJKLM", true},
		{"valid all digits 2-9", "234567892345", true},
		{"valid mixed", "A2B3C4D5E6F7", true},
		{"too short", "ABCDEFGHJKL", false},
		{"too long", "ABCDEFGHJKLMN", false},
		{"contains excluded letter I", "IBCDEFGHJKLM", false},
		{"contains excluded letter O", "OBCDEFGHJKLM", false},
		{"contains excluded digit 0", "0BCDEFGHJKLM", false},
		{"contains excluded digit 1", "1BCDEFGHJKLM", false},
		{"lowercase rejected (caller must ToUpper first)", "abcdefghjklm", false},
		{"empty", "", false},
		{"12-byte utf8 non-ascii", "ÄÄÄÄÄÄ", false}, // 6×2 bytes = 12 bytes, bytes out of charset
		{"ascii punctuation", "ABCDEFGHJK.M", false},
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
