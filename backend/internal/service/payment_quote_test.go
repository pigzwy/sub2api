//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestQuoteUsesFXDistinguishesBalanceAndSubscription(t *testing.T) {
	t.Parallel()

	if !quoteUsesFX(payment.OrderTypeBalance, payment.TypeInfini, "USD", 6.67) {
		t.Fatal("Infini balance should convert when rate > 0")
	}
	if !quoteUsesFX(payment.OrderTypeBalance, payment.TypeInfini, "CNY", 6.67) {
		t.Fatal("Infini labeled CNY must still convert RMB packages")
	}
	if quoteUsesFX(payment.OrderTypeBalance, payment.TypeAlipay, "CNY", 6.67) {
		t.Fatal("Alipay balance should not convert")
	}
	if quoteUsesFX(payment.OrderTypeBalance, payment.TypeStripe, "USD", 6.67) {
		t.Fatal("Stripe balance should not convert")
	}
	if !quoteUsesFX(payment.OrderTypeSubscription, payment.TypeAlipay, "CNY", 6.67) {
		t.Fatal("CNY subscription should convert when rate > 0")
	}
	if quoteUsesFX(payment.OrderTypeSubscription, payment.TypeInfini, "USD", 6.67) {
		t.Fatal("USD subscription should keep plan price")
	}
	if quoteUsesFX(payment.OrderTypeBalance, payment.TypeInfini, "USD", 0) {
		t.Fatal("rate 0 must disable conversion")
	}
}

func TestQuoteOrderForcesInfiniUSDForLeftoverInstanceCurrency(t *testing.T) {
	t.Parallel()

	t.Run("currency=CNY", func(t *testing.T) {
		t.Parallel()
		quote := quoteInfiniBalance(t, `{"currency":"CNY"}`)
		require.Equal(t, "USD", quote.Currency)
		require.Equal(t, "7.50", quote.PayAmount)
		require.InDelta(t, 7.5, quote.PayAmountValue, 1e-9)
		require.InDelta(t, 6.67, quote.FxRate, 1e-9)
		require.True(t, quote.FxConverted)
		require.Equal(t, "50.00", quote.CreditAmount)
	})

	t.Run("currency empty", func(t *testing.T) {
		t.Parallel()
		quote := quoteInfiniBalance(t, `{}`)
		require.Equal(t, "USD", quote.Currency)
		require.Equal(t, "7.50", quote.PayAmount)
		require.InDelta(t, 6.67, quote.FxRate, 1e-9)
	})
}

func TestQuoteOrderMatchesCreateOrderSnapshotForInfiniCNYInstance(t *testing.T) {
	t.Parallel()

	quote := quoteInfiniBalance(t, `{"currency":"CNY"}`)
	cfg := &PaymentConfig{USDTUSDToCNYRate: 6.67}
	req := CreateOrderRequest{PaymentType: payment.TypeInfini, OrderType: payment.OrderTypeBalance}
	sel := &payment.InstanceSelection{
		ProviderKey: payment.TypeInfini,
		Config:      map[string]string{"currency": "CNY"},
	}
	snap := newPaymentOrderFinancialSnapshot(req, cfg, sel, 50, 50, 0, quote.PayAmountValue)

	require.Equal(t, quote.Currency, snap.Currency)
	require.Equal(t, quote.PayAmount, snap.PayAmount)
	require.Equal(t, decimalAmountString(quote.FxRate, 4), snap.FxRate)
	require.Equal(t, quote.FxConverted, snap.FxConverted)
	require.Equal(t, "USD", snap.Currency)
	require.Equal(t, "7.50", snap.PayAmount)
}

func TestQuoteOrderForcesInfiniUSDWhenConsistencyFallsBackToCNY(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	enabled := true
	usdtRate := 6.67
	settings := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingMinRechargeAmount: "1",
	}}
	configService := &PaymentConfigService{entClient: client, settingRepo: settings}
	require.NoError(t, configService.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{
		Enabled:          &enabled,
		USDTUSDToCNYRate: &usdtRate,
	}))

	methodCurrency, err := configService.ValidateMethodCurrencyConsistency(ctx, payment.TypeInfini)
	require.NoError(t, err)
	require.Equal(t, payment.DefaultPaymentCurrency, methodCurrency)

	quote, err := (&PaymentService{configService: configService}).QuoteOrder(ctx, CreateOrderRequest{
		Amount:      50,
		PaymentType: payment.TypeInfini,
		OrderType:   payment.OrderTypeBalance,
	})
	require.NoError(t, err)
	require.Equal(t, "USD", quote.Currency)
	require.Equal(t, "7.50", quote.PayAmount)
	require.InDelta(t, 6.67, quote.FxRate, 1e-9)
}

func TestQuoteOrderKeepsAlipayStripeAirwallexCurrencies(t *testing.T) {
	t.Parallel()

	alipay := quoteBalanceForInstance(t, payment.TypeAlipay, payment.TypeAlipay, `{"currency":"USD"}`)
	require.Equal(t, payment.DefaultPaymentCurrency, alipay.Currency)
	require.Equal(t, "50.00", alipay.PayAmount)
	require.False(t, alipay.FxConverted)
	require.InDelta(t, 0, alipay.FxRate, 1e-9)

	stripe := quoteBalanceForInstance(t, payment.TypeStripe, payment.TypeStripe, `{"currency":"USD"}`)
	require.Equal(t, "USD", stripe.Currency)
	require.Equal(t, "50.00", stripe.PayAmount)
	require.False(t, stripe.FxConverted)

	airwallex := quoteBalanceForInstance(t, payment.TypeAirwallex, payment.TypeAirwallex, `{"currency":"HKD"}`)
	require.Equal(t, "HKD", airwallex.Currency)
	require.Equal(t, "50.00", airwallex.PayAmount)
	require.False(t, airwallex.FxConverted)
}

func quoteInfiniBalance(t *testing.T, config string) *QuoteOrderResponse {
	t.Helper()
	return quoteBalanceForInstance(t, payment.TypeInfini, payment.TypeInfini, config)
}

func quoteBalanceForInstance(t *testing.T, providerKey, supportedTypes, config string) *QuoteOrderResponse {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(providerKey).
		SetName(providerKey + " leftover").
		SetConfig(config).
		SetSupportedTypes(supportedTypes).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	enabled := true
	usdtRate := 6.67
	settings := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingMinRechargeAmount: "1",
	}}
	configService := &PaymentConfigService{entClient: client, settingRepo: settings}
	require.NoError(t, configService.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{
		Enabled:          &enabled,
		USDTUSDToCNYRate: &usdtRate,
	}))

	quote, err := (&PaymentService{configService: configService}).QuoteOrder(ctx, CreateOrderRequest{
		Amount:      50,
		PaymentType: providerKey,
		OrderType:   payment.OrderTypeBalance,
	})
	require.NoError(t, err)
	require.NotNil(t, quote)
	return quote
}
