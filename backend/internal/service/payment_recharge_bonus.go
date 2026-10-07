package service

import (
	"encoding/json"
	"math"
	"sort"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	RechargeBonusFixed      = "fixed"
	RechargeBonusPercentage = "percentage"
	maxRechargeBonusTiers   = 20
)

type RechargeBonusTier struct {
	MinAmount    float64 `json:"min_amount"`
	BonusPercent float64 `json:"bonus_percent"`
}

// NormalizeRechargeBonusTiers validates before decimal conversion and never
// silently drops malformed rows. Thresholds are original package CNY amounts.
func NormalizeRechargeBonusTiers(tiers []RechargeBonusTier) ([]RechargeBonusTier, error) {
	invalid := func() error {
		return infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", "use at most 20 unique non-negative thresholds, percentages 0–1000, and at most 2 decimal places")
	}
	if len(tiers) > maxRechargeBonusTiers {
		return nil, invalid()
	}
	out := make([]RechargeBonusTier, 0, len(tiers))
	seen := make(map[float64]bool, len(tiers))
	for _, tier := range tiers {
		for _, value := range []float64{tier.MinAmount, tier.BonusPercent} {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return nil, invalid()
			}
			d := decimal.NewFromFloat(value)
			if !d.Equal(d.Round(2)) {
				return nil, invalid()
			}
		}
		if tier.MinAmount > 9999999999.99 || tier.BonusPercent > 1000 || seen[tier.MinAmount] {
			return nil, invalid()
		}
		seen[tier.MinAmount] = true
		out = append(out, tier)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MinAmount < out[j].MinAmount })
	return out, nil
}

func rechargeBonusMode(mode string) string {
	if mode == "" {
		return RechargeBonusFixed
	}
	return mode
}

func parseRechargeBonusTiers(raw string) ([]RechargeBonusTier, error) {
	if raw == "" {
		return []RechargeBonusTier{}, nil
	}
	var tiers []RechargeBonusTier
	if err := json.Unmarshal([]byte(raw), &tiers); err != nil {
		return nil, err
	}
	return NormalizeRechargeBonusTiers(tiers)
}

type balanceRechargeCredit struct {
	Credit  float64
	Bonus   float64
	Percent float64
	Mode    string
}

// Fixed keeps the historical single final rounding. Percentage rounds base USD
// credit, then its gift to cents. Gifts never affect gateway pay amounts or FX.
func calculateBalanceRechargeCredit(amount float64, cfg *PaymentConfig) (balanceRechargeCredit, error) {
	result := balanceRechargeCredit{Mode: rechargeBonusMode(cfg.RechargeBonusMode)}
	switch result.Mode {
	case RechargeBonusFixed:
		result.Credit = calculateCreditedBalance(amount, cfg.BalanceRechargeMultiplier, cfg.BalanceRechargePackages)
		if pkg, ok := FindRechargePackageByAmount(cfg.BalanceRechargePackages, amount); ok {
			result.Bonus = pkg.Bonus
		}
	case RechargeBonusPercentage:
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
			return result, infraerrors.BadRequest("INVALID_AMOUNT", "recharge amount must be positive and finite")
		}
		if d := decimal.NewFromFloat(amount); !d.Equal(d.Round(2)) {
			return result, infraerrors.BadRequest("INVALID_AMOUNT", "percentage recharge amount allows at most 2 decimal places")
		}
		if cfg.rechargeBonusConfigErr != nil {
			return result, infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", "stored recharge bonus tiers are invalid; save valid tiers before recharging")
		}
		tiers, err := NormalizeRechargeBonusTiers(cfg.RechargeBonusTiers)
		if err != nil {
			return result, err
		}
		for _, tier := range tiers {
			if decimal.NewFromFloat(amount).LessThan(decimal.NewFromFloat(tier.MinAmount)) {
				break
			}
			result.Percent = tier.BonusPercent
		}
		base := decimal.NewFromFloat(amount).Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(cfg.BalanceRechargeMultiplier))).Round(2)
		gift := base.Mul(decimal.NewFromFloat(result.Percent)).Div(decimal.NewFromInt(100)).Round(2)
		result.Bonus = gift.InexactFloat64()
		result.Credit = base.Add(gift).InexactFloat64()
	default:
		return result, infraerrors.BadRequest("INVALID_RECHARGE_BONUS_MODE", "recharge bonus mode must be fixed or percentage")
	}
	if result.Mode == RechargeBonusPercentage && (math.IsInf(result.Credit, 0) || math.IsNaN(result.Credit) || result.Credit <= 0 || result.Credit > 9999999999.99) {
		return result, infraerrors.BadRequest("INVALID_AMOUNT", "credited amount is outside the supported range")
	}
	return result, nil
}

func BuildCheckoutRechargePackagesForConfig(cfg *PaymentConfig) ([]CheckoutRechargePackage, error) {
	packages := cfg.BalanceRechargePackages
	if len(packages) == 0 {
		packages = DefaultRechargePackages()
	}
	out := BuildCheckoutRechargePackages(packages, cfg.BalanceRechargeMultiplier)
	for i, pkg := range packages {
		credit, err := calculateBalanceRechargeCredit(pkg.Amount, cfg)
		if err != nil {
			return nil, err
		}
		out[i].Credit, out[i].Bonus = credit.Credit, credit.Bonus
	}
	return out, nil
}
