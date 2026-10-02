package service

import (
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestParseRechargePackagesFallsBackToDefaults(t *testing.T) {
	t.Parallel()

	got := ParseRechargePackages("")
	if len(got) != len(DefaultRechargePackages()) {
		t.Fatalf("empty setting length = %d, want defaults", len(got))
	}
	got = ParseRechargePackages("{")
	if len(got) != len(DefaultRechargePackages()) {
		t.Fatalf("invalid json length = %d, want defaults", len(got))
	}
}

func TestNormalizeRechargePackagesRejectsDuplicateAmounts(t *testing.T) {
	t.Parallel()

	_, err := NormalizeRechargePackages([]RechargePackage{
		{ID: "a", Amount: 50, Name: "体验"},
		{ID: "b", Amount: 50.001, Name: "标准"},
	})
	if err == nil {
		t.Fatal("expected duplicate amount to fail")
	}
	if appErr := infraerrors.FromError(err); appErr.Reason != "INVALID_RECHARGE_PACKAGES" {
		t.Fatalf("reason = %q, want INVALID_RECHARGE_PACKAGES", appErr.Reason)
	}
}

func TestCalculateCreditedBalanceUsesPackageBonusWhenConfigured(t *testing.T) {
	t.Parallel()

	packages := []RechargePackage{
		{ID: "starter", Amount: 50, Bonus: 0, Name: "体验"},
		{ID: "standard", Amount: 100, Bonus: 2.99, Name: "标准"},
	}

	got := calculateCreditedBalance(50, 1.05, packages)
	if got != 52.5 {
		t.Fatalf("starter credit = %v, want 52.5 (rate plus zero bonus)", got)
	}
	got = calculateCreditedBalance(100, 1.05, packages)
	if got != 107.99 {
		t.Fatalf("standard credit = %v, want 107.99", got)
	}
	got = calculateCreditedBalance(80, 1.05, packages)
	if got != 84 {
		t.Fatalf("unmatched amount credit = %v, want multiplier 84", got)
	}
}

func TestCalculateCreditedBalanceKeepsMultiplierWhenAnotherPackageHasBonus(t *testing.T) {
	t.Parallel()

	packages := []RechargePackage{
		{ID: "cny100", Amount: 100, Bonus: 0, Name: "基础"},
		{ID: "cny200", Amount: 200, Bonus: 5, Name: "加赠"},
	}

	got := calculateCreditedBalance(100, 0.14, packages)
	if got != 14 {
		t.Fatalf("zero-bonus CNY package credit = %v, want 14", got)
	}
	got = calculateCreditedBalance(200, 0.14, packages)
	if got != 33 {
		t.Fatalf("bonus CNY package credit = %v, want 33", got)
	}
}

func TestCalculateCreditedBalanceKeepsMultiplierWhenNoPackageBonus(t *testing.T) {
	t.Parallel()

	packages := DefaultRechargePackages()
	got := calculateCreditedBalance(50, 1.05, packages)
	if got != 52.5 {
		t.Fatalf("default package credit = %v, want multiplier 52.5", got)
	}
}

func TestBuildCheckoutRechargePackagesIncludesComputedCredit(t *testing.T) {
	t.Parallel()

	packages := []RechargePackage{
		{ID: "starter", Amount: 50, Bonus: 0, Name: "体验", NameEn: "Starter"},
		{ID: "standard", Amount: 100, Bonus: 2.99, Name: "标准", NameEn: "Standard"},
	}
	got := BuildCheckoutRechargePackages(packages, 1.05)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Credit != 52.5 || got[0].Bonus != 0 {
		t.Fatalf("starter checkout = %+v, want credit 52.5 bonus 0", got[0])
	}
	if got[1].Credit != 107.99 || got[1].Bonus != 2.99 {
		t.Fatalf("standard checkout = %+v, want credit 107.99 bonus 2.99", got[1])
	}
}
