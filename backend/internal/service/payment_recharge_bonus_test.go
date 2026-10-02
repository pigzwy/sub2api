//go:build unit

package service

import (
	"context"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestRechargeBonusFixedCompatibility(t *testing.T) {
	packages := []RechargePackage{{ID: "test", Amount: 200, Bonus: 5, Name: "Test"}}
	for _, mode := range []string{"", RechargeBonusFixed} {
		for _, multiplier := range []float64{0.14, 0.142857, 1, 1.234567} {
			for _, amount := range []float64{50, 200, 200.001, 250.55} {
				cfg := &PaymentConfig{BalanceRechargeMultiplier: multiplier, BalanceRechargePackages: packages, RechargeBonusMode: mode,
					RechargeBonusTiers: []RechargeBonusTier{{MinAmount: 0, BonusPercent: 1000}}}
				credit, err := calculateBalanceRechargeCredit(amount, cfg)
				require.NoError(t, err)
				require.Equal(t, calculateCreditedBalance(amount, multiplier, packages), credit.Credit)
				require.Equal(t, RechargeBonusFixed, credit.Mode)
			}
		}
	}
}

func TestRechargeBonusPercentageThresholdsAndRounding(t *testing.T) {
	cfg := &PaymentConfig{BalanceRechargeMultiplier: 0.14, RechargeBonusMode: RechargeBonusPercentage,
		BalanceRechargePackages: []RechargePackage{{ID: "test", Amount: 200, Bonus: 500, Name: "Test"}},
		RechargeBonusTiers:      []RechargeBonusTier{{MinAmount: 200, BonusPercent: 20}, {MinAmount: 50, BonusPercent: 5}, {MinAmount: 300, BonusPercent: 0}}}
	for _, tc := range []struct{ amount, credit, bonus, percent float64 }{
		{49.99, 7, 0, 0}, {50, 7.35, 0.35, 5}, {199.99, 29.4, 1.4, 5},
		{200, 33.6, 5.6, 20}, {250, 42, 7, 20}, {300, 42, 0, 0},
	} {
		actual, err := calculateBalanceRechargeCredit(tc.amount, cfg)
		require.NoError(t, err)
		require.Equal(t, tc.credit, actual.Credit)
		require.Equal(t, tc.bonus, actual.Bonus)
		require.Equal(t, tc.percent, actual.Percent)
	}
	cfg.BalanceRechargeMultiplier = 0.3335
	cfg.RechargeBonusTiers = []RechargeBonusTier{{MinAmount: 0, BonusPercent: 50}}
	actual, err := calculateBalanceRechargeCredit(1, cfg)
	require.NoError(t, err)
	require.Equal(t, 0.17, actual.Bonus) // base rounds to .33, gift .165 rounds to .17
	require.Equal(t, 0.50, actual.Credit)
	cfg.RechargeBonusTiers = nil
	actual, err = calculateBalanceRechargeCredit(200, cfg)
	require.NoError(t, err)
	require.Zero(t, actual.Bonus) // fixed gift is never used in percentage mode
	require.Equal(t, 66.70, actual.Credit)
	for _, amount := range []float64{math.NaN(), math.Inf(1), -1, 0, 100.001, 1e20} {
		_, err = calculateBalanceRechargeCredit(amount, cfg)
		require.Error(t, err)
	}
}

func TestRechargeBonusTierValidation(t *testing.T) {
	for _, tiers := range [][]RechargeBonusTier{
		{{MinAmount: -1}}, {{MinAmount: math.NaN()}}, {{MinAmount: math.Inf(1)}},
		{{MinAmount: 0.001}}, {{MinAmount: 1e20}}, {{BonusPercent: -1}},
		{{BonusPercent: math.NaN()}}, {{BonusPercent: math.Inf(1)}}, {{BonusPercent: 1000.01}},
		{{BonusPercent: 0.001}}, {{MinAmount: 50}, {MinAmount: 50}}, make([]RechargeBonusTier, 21),
	} {
		_, err := NormalizeRechargeBonusTiers(tiers)
		require.Error(t, err)
	}
	tiers, err := NormalizeRechargeBonusTiers([]RechargeBonusTier{{MinAmount: 200.01, BonusPercent: 1000}, {MinAmount: 50, BonusPercent: 0.29}})
	require.NoError(t, err)
	require.Equal(t, 50.0, tiers[0].MinAmount)
}

func TestRechargeBonusConfigPreservesBothModes(t *testing.T) {
	ctx := context.Background()
	repo := &paymentConfigSettingRepoStub{values: map[string]string{}}
	svc := &PaymentConfigService{settingRepo: repo}
	packages := []RechargePackage{{ID: "test", Amount: 200, Bonus: 5, Name: "Test"}}
	tiers := []RechargeBonusTier{{MinAmount: 200, BonusPercent: 20}}
	require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{BalanceRechargePackages: &packages, RechargeBonusTiers: &tiers}))
	cfg, err := svc.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, RechargeBonusFixed, cfg.RechargeBonusMode) // storing tiers never implicitly enables them
	for _, mode := range []string{RechargeBonusPercentage, RechargeBonusFixed} {
		require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusMode: &mode}))
		cfg, err = svc.GetPaymentConfig(ctx)
		require.NoError(t, err)
		require.Equal(t, mode, cfg.RechargeBonusMode)
		require.Equal(t, packages, cfg.BalanceRechargePackages)
		require.Equal(t, tiers, cfg.RechargeBonusTiers)
	}
	invalidMode := "discount"
	require.Error(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusMode: &invalidMode}))
	invalidTiers := []RechargeBonusTier{{BonusPercent: -1}}
	percentage := RechargeBonusPercentage
	require.Error(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusMode: &percentage, RechargeBonusTiers: &invalidTiers}))
	cfg, err = svc.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, RechargeBonusFixed, cfg.RechargeBonusMode) // invalid patch does not partially activate
	require.Equal(t, tiers, cfg.RechargeBonusTiers)
	for _, raw := range []string{"broken", `[{"bonus_percent":-1}]`} {
		cfg = svc.parsePaymentConfig(map[string]string{SettingRechargeBonusMode: RechargeBonusPercentage, SettingRechargeBonusTiers: raw})
		_, err = calculateBalanceRechargeCredit(200, cfg)
		require.Error(t, err) // no fallback to fixed gifts or silent zero when storage is corrupt
	}
}

func TestRechargeBonusDoesNotChangeGatewayAmounts(t *testing.T) {
	for _, method := range []string{payment.TypeInfini, payment.TypeAlipay, payment.TypeStripe, payment.TypeAirwallex} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			config := &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
				SettingPaymentEnabled: "true", SettingBalanceRechargeMult: "0.14", SettingUSDTUSDToCNYRate: "6.67",
				SettingRechargeFeeRate: "2", SettingRechargeBonusTiers: `[{"min_amount":50,"bonus_percent":20}]`,
			}}}
			svc := &PaymentService{configService: config}
			req := CreateOrderRequest{Amount: 50, PaymentType: method, OrderType: payment.OrderTypeBalance}
			fixed, err := svc.QuoteOrder(ctx, req)
			require.NoError(t, err)
			mode := RechargeBonusPercentage
			require.NoError(t, config.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusMode: &mode}))
			percentage, err := svc.QuoteOrder(ctx, req)
			require.NoError(t, err)
			require.Equal(t, "8.40", percentage.CreditAmount)
			require.Equal(t, "1.40", percentage.BonusAmount)
			require.Equal(t, fixed.PayAmount, percentage.PayAmount)
			require.Equal(t, fixed.FeeAmount, percentage.FeeAmount)
			require.Equal(t, fixed.FxRate, percentage.FxRate)
			require.Equal(t, fixed.Currency, percentage.Currency)
			if method == payment.TypeInfini {
				require.Equal(t, "7.65", percentage.PayAmount)
			}
			cfg, err := config.GetPaymentConfig(ctx)
			require.NoError(t, err)
			cards, err := BuildCheckoutRechargePackagesForConfig(cfg)
			require.NoError(t, err)
			require.Equal(t, 8.40, cards[0].Credit)
			require.Equal(t, 1.40, cards[0].Bonus)
			// Subscription quoting bypasses even malformed balance bonus settings.
			cfg.RechargeBonusMode = "invalid"
			sub, err := svc.quoteOrderAmounts(ctx, CreateOrderRequest{OrderType: payment.OrderTypeSubscription, PaymentType: method}, cfg, 50, 50, nil)
			require.NoError(t, err)
			require.Equal(t, "50.00", sub.CreditAmount)
			require.Equal(t, "0.00", sub.BonusAmount)
		})
	}
}
