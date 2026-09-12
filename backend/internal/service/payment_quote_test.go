//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestQuoteUsesFXDistinguishesBalanceAndSubscription(t *testing.T) {
	t.Parallel()

	if !quoteUsesFX(payment.OrderTypeBalance, payment.TypeInfini, "USD", 6.67) {
		t.Fatal("Infini balance should convert when rate > 0")
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
