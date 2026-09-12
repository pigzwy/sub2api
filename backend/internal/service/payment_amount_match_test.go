//go:build unit

package service

import "testing"

func TestPaymentAmountsMatchUsesMinorUnits(t *testing.T) {
	t.Parallel()

	if err := paymentAmountsMatch("7.50", 0, "7.5", 0, "USD"); err != nil {
		t.Fatalf("equivalent USD strings should match: %v", err)
	}
	if err := paymentAmountsMatch("7.50", 0, "7.49", 0, "USD"); err == nil {
		t.Fatal("expected underpay to fail")
	}
	if err := paymentAmountsMatch("7.51", 0, "7.50", 0, "USD"); err == nil {
		t.Fatal("expected overpay to fail")
	}
	if err := paymentAmountsMatch("50.00", 0, "50", 0, "CNY"); err != nil {
		t.Fatalf("equivalent CNY strings should match: %v", err)
	}
	if err := paymentAmountsMatch("7.50", 0, "7.50", 0, "USDT"); err != nil {
		t.Fatalf("USDT should use 2 decimal minor units: %v", err)
	}
	if err := paymentAmountsMatch("", 0, "7.50", 0, "USD"); err == nil {
		t.Fatal("expected missing paid amount to fail")
	}
}
