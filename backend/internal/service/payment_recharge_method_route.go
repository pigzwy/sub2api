package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const easyPayTenYuanAmount = 10

func isEasyPayTenYuanAmount(amount float64) bool {
	return decimal.NewFromFloat(amount).Round(2).Equal(decimal.NewFromInt(easyPayTenYuanAmount))
}

func (s *PaymentService) balanceQuoteProviderKey(ctx context.Context, paymentType string) (string, error) {
	if s == nil || s.configService == nil {
		return "", nil
	}
	inst, err := s.configService.resolveEnabledVisibleMethodInstance(ctx, paymentType)
	if err != nil || inst == nil {
		return "", err
	}
	return strings.TrimSpace(inst.ProviderKey), nil
}

func validateBalanceRechargeMethodRoute(amount float64, providerKey, paymentType string) error {
	if balanceRechargeMethodAllowed(amount, providerKey, paymentType) {
		return nil
	}
	return infraerrors.BadRequest("PAYMENT_METHOD_AMOUNT_MISMATCH", "this recharge amount is not available for the selected payment method")
}

// balanceAmountBelowGlobalMin keeps the admin recharge floor, except the ¥10
// EasyPay card. The provider check still rejects that amount on every other rail.
func balanceAmountBelowGlobalMin(amount, minAmount float64) bool {
	if minAmount <= 0 || isEasyPayTenYuanAmount(amount) {
		return false
	}
	return amount < minAmount
}

func balanceRechargeMethodAllowed(amount float64, providerKey, paymentType string) bool {
	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	easyPay := providerKey == string(payment.TypeEasyPay)
	if isEasyPayTenYuanAmount(amount) {
		if providerKey != "" {
			return easyPay
		}
		// Quotes may not have selected an instance yet. EasyPay checkout buttons
		// are alipay/wxpay (or a custom EasyPay type), so only Stripe, Infini,
		// Airwallex, and USDT are rejected here. CreateOrder checks the provider
		// that was actually selected.
		return !isNonEasyPayBalanceProviderType(paymentType)
	}
	if easyPay || strings.EqualFold(strings.TrimSpace(paymentType), string(payment.TypeEasyPay)) {
		return false
	}
	return true
}

func isNonEasyPayBalanceProviderType(paymentType string) bool {
	base := strings.ToLower(strings.TrimSpace(payment.GetBasePaymentType(paymentType)))
	if base == string(payment.TypeStripe) || base == string(payment.TypeInfini) || base == string(payment.TypeAirwallex) {
		return true
	}
	return isUsdtBalancePaymentType(paymentType)
}
