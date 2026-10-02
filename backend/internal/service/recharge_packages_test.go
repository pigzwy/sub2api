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

func TestLegacyPackageBonusDoesNotStackWithUpstreamPromotion(t *testing.T) {
	t.Parallel()
	packages := []RechargePackage{
		{ID: "starter", Amount: 50, Name: "Starter"},
		{ID: "standard", Amount: 100, Bonus: 99, Name: "Standard"},
	}
	cfg := &PaymentConfig{BalanceRechargeMultiplier: 1.05,
		RechargeBonusTiers: []RechargeBonusTier{{MinAmount: 100, BonusPercent: 10}},
	}
	got := BuildCheckoutRechargePackages(packages, cfg)
	if got[0].Credit != 52.5 || got[0].Bonus != 0 {
		t.Fatalf("starter checkout = %+v, want credit 52.5 bonus 0", got[0])
	}
	if got[1].Credit != 115.5 || got[1].Bonus != 10.5 {
		t.Fatalf("standard checkout = %+v, want upstream credit 115.5 bonus 10.5", got[1])
	}
	normalized, err := NormalizeRechargePackages(packages)
	if err != nil || normalized[1].Bonus != 0 {
		t.Fatalf("legacy bonus must not remain editable: %+v, %v", normalized, err)
	}
	cfg.RechargeBonusMode = RechargeBonusModeDiscount
	got = BuildCheckoutRechargePackages(packages, cfg)
	if got[1].Credit != 105 || got[1].Bonus != 0 {
		t.Fatalf("discount checkout = %+v, discount must not be shown as extra credit", got[1])
	}
}
