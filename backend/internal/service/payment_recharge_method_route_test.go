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

func TestCheckoutUsesOnlyConfiguredRechargeCards(t *testing.T) {
	cfg := &PaymentConfig{
		BalanceRechargeMultiplier: 0.14,
		BalanceRechargePackages: []RechargePackage{
			{ID: "starter", Amount: 50, Bonus: 1, Name: "体验"},
			{ID: "ten", Amount: 10, Bonus: 0, Name: "十元"},
		},
	}
	cards, err := BuildCheckoutRechargePackagesForConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"starter", "ten"}, []string{cards[0].ID, cards[1].ID})
	require.InDelta(t, 8, cards[0].Credit, 1e-9)
	require.InDelta(t, 1.4, cards[1].Credit, 1e-9)

	withoutTen := &PaymentConfig{
		BalanceRechargeMultiplier: 1,
		BalanceRechargePackages:   []RechargePackage{{ID: "starter", Amount: 50, Name: "体验"}},
	}
	cards, err = BuildCheckoutRechargePackagesForConfig(withoutTen)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.Equal(t, "starter", cards[0].ID)
}
