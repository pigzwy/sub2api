package service

import (
	"math"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const defaultBalanceRechargeMultiplier = 1.0

func normalizeBalanceRechargeMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return defaultBalanceRechargeMultiplier
	}
	return multiplier
}

const (
	minUsableSubscriptionUSDToCNYRate = 0.0001
	maxUsableSubscriptionUSDToCNYRate = 1000000
)

// validateSubscriptionUSDToCNYRate rejects NaN/Inf/negative/out-of-range rates.
// 0 remains valid and means FX conversion is disabled.
func validateSubscriptionUSDToCNYRate(rate float64) error {
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return infraerrors.BadRequest("INVALID_FX_RATE", "USD/CNY rate is invalid")
	}
	if rate < 0 {
		return infraerrors.BadRequest("INVALID_FX_RATE", "USD/CNY rate must be 0 or a positive number")
	}
	if rate > 0 && (rate < minUsableSubscriptionUSDToCNYRate || rate > maxUsableSubscriptionUSDToCNYRate) {
		return infraerrors.BadRequest("INVALID_FX_RATE", "USD/CNY rate is out of range")
	}
	return nil
}

// normalizeSubscriptionUSDToCNYRate 将非法或超范围值归一为 0（换算关闭）。
// 0 表示：订阅按 price 直付；Infini/USDT 余额充值也不把套餐换成美元实付。
// CreateOrder/Quote 必须先走 validateSubscriptionUSDToCNYRate，避免把非法汇率静默当成 0。
func normalizeSubscriptionUSDToCNYRate(rate float64) float64 {
	if err := validateSubscriptionUSDToCNYRate(rate); err != nil {
		return 0
	}
	return rate
}

// calculateCreditedBalance converts the paid amount with the recharge rate, then
// adds that package's USD bonus when the amount matches a configured card.
func calculateCreditedBalance(paymentAmount, multiplier float64, packages []RechargePackage) float64 {
	credit := decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier)))
	if pkg, ok := FindRechargePackageByAmount(packages, paymentAmount); ok {
		credit = credit.Add(decimal.NewFromFloat(pkg.Bonus))
	}
	return credit.Round(2).InexactFloat64()
}

func calculateGatewayRefundAmount(orderAmount, payAmount, refundAmount float64, currency string) float64 {
	if orderAmount <= 0 || payAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	fractionDigits := int32(payment.CurrencyMaxFractionDigits(currency))
	if math.Abs(refundAmount-orderAmount) <= paymentAmountToleranceForCurrency(currency) {
		return decimal.NewFromFloat(payAmount).Round(fractionDigits).InexactFloat64()
	}
	return decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(refundAmount)).
		Div(decimal.NewFromFloat(orderAmount)).
		Round(fractionDigits).
		InexactFloat64()
}
