package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestEasyPayTenYuanRoutesOnlyToEasyPay(t *testing.T) {
	require.NoError(t, validateBalanceRechargeMethodRoute(10, payment.TypeEasyPay, payment.TypeAlipay))
	require.NoError(t, validateBalanceRechargeMethodRoute(10, "", payment.TypeAlipay))
	require.NoError(t, validateBalanceRechargeMethodRoute(10, "", payment.TypeWxpay))
	require.Error(t, validateBalanceRechargeMethodRoute(10, "", payment.TypeStripe))
	require.Error(t, validateBalanceRechargeMethodRoute(10, "", payment.TypeInfini))
	require.Error(t, validateBalanceRechargeMethodRoute(10, "", payment.TypeAirwallex))
	require.Error(t, validateBalanceRechargeMethodRoute(10, payment.TypeStripe, payment.TypeStripe))
	require.Error(t, validateBalanceRechargeMethodRoute(10, payment.TypeInfini, payment.TypeInfini))
	require.Error(t, validateBalanceRechargeMethodRoute(10, payment.TypeAlipay, payment.TypeAlipay))
	require.Error(t, validateBalanceRechargeMethodRoute(50, payment.TypeEasyPay, payment.TypeAlipay))
	require.Error(t, validateBalanceRechargeMethodRoute(50, "", payment.TypeEasyPay))
	require.NoError(t, validateBalanceRechargeMethodRoute(50, payment.TypeStripe, payment.TypeStripe))
	require.NoError(t, validateBalanceRechargeMethodRoute(100, payment.TypeInfini, payment.TypeInfini))
	require.NoError(t, validateBalanceRechargeMethodRoute(100, payment.TypeAlipay, payment.TypeAlipay))
	require.NoError(t, validateBalanceRechargeMethodRoute(100, payment.TypeAirwallex, payment.TypeAirwallex))
	require.False(t, balanceAmountBelowGlobalMin(10, 50))
	require.True(t, balanceAmountBelowGlobalMin(20, 50))
	require.False(t, balanceAmountBelowGlobalMin(50, 50))
}

func TestEnsureEasyPayTenPackageKeepsAnExistingTenYuanCard(t *testing.T) {
	packages := []RechargePackage{{ID: "custom-10", Amount: 10, Name: "十元"}}
	got := ensureEasyPayTenPackage(packages)
	require.Equal(t, packages, got)

	withStarter := ensureEasyPayTenPackage([]RechargePackage{{ID: "starter", Amount: 50, Name: "体验"}})
	require.Equal(t, 10.0, withStarter[0].Amount)
	require.Equal(t, "easypay-10", withStarter[0].ID)
	require.Equal(t, 50.0, withStarter[1].Amount)
}

func TestCheckoutAddsTenYuanCardWithoutPersistingIt(t *testing.T) {
	cfg := &PaymentConfig{
		BalanceRechargeMultiplier: 0.14,
		BalanceRechargePackages:   []RechargePackage{{ID: "starter", Amount: 50, Bonus: 1, Name: "体验"}},
	}
	cards, err := BuildCheckoutRechargePackagesForConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, "easypay-10", cards[0].ID)
	require.InDelta(t, 1.4, cards[0].Credit, 1e-9)
	require.InDelta(t, 0, cards[0].Bonus, 1e-9)
	require.InDelta(t, 8, cards[1].Credit, 1e-9)
	require.Len(t, cfg.BalanceRechargePackages, 1)
	require.InDelta(t, 50, cfg.BalanceRechargePackages[0].Amount, 1e-9)
}
