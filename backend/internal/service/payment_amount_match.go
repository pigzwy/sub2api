package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

func formatPaymentAmountExact(amount float64, currency string) string {
	digits := int32(payment.CurrencyMaxFractionDigits(currency))
	return decimal.NewFromFloat(amount).Round(digits).StringFixed(digits)
}

func snapshotPayAmountExact(orderPayAmount float64, currency string, snapshot *paymentOrderProviderSnapshot) string {
	if snapshot != nil {
		if exact := strings.TrimSpace(snapshot.PayAmount); exact != "" {
			return exact
		}
	}
	return formatPaymentAmountExact(orderPayAmount, currency)
}

// paymentAmountsMatch compares provider-paid and expected amounts in minor units.
// Overpay and underpay both fail. Float values are only used when an exact
// decimal string is missing, and are immediately converted to currency precision.
func paymentAmountsMatch(paidExact string, paid float64, expectedExact string, expected float64, currency string) error {
	paidStr := strings.TrimSpace(paidExact)
	if paidStr == "" {
		if !isValidProviderAmount(paid) {
			return fmt.Errorf("invalid paid amount from provider: %v", paid)
		}
		paidStr = formatPaymentAmountExact(paid, currency)
	}
	expectedStr := strings.TrimSpace(expectedExact)
	if expectedStr == "" {
		if !isValidProviderAmount(expected) {
			return fmt.Errorf("invalid expected payment amount: %v", expected)
		}
		expectedStr = formatPaymentAmountExact(expected, currency)
	}

	paidMinor, err := payment.AmountToMinorUnit(paidStr, currency)
	if err != nil {
		return fmt.Errorf("invalid paid amount %s: %w", paidStr, err)
	}
	expectedMinor, err := payment.AmountToMinorUnit(expectedStr, currency)
	if err != nil {
		return fmt.Errorf("invalid expected amount %s: %w", expectedStr, err)
	}
	if paidMinor != expectedMinor {
		return fmt.Errorf("amount mismatch: expected %s, got %s", expectedStr, paidStr)
	}
	return nil
}
