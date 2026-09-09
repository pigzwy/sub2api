package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	maxRechargePackages      = 24
	maxRechargePackageName   = 32
	maxRechargePackageDesc   = 80
	rechargePackageBadgePop  = "popular"
	rechargePackageBadgeBest = "bestValue"
)

var rechargePackageIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// RechargePackage is an admin-configured balance top-up card.
type RechargePackage struct {
	ID            string  `json:"id"`
	Amount        float64 `json:"amount"`
	Bonus         float64 `json:"bonus"`
	Badge         string  `json:"badge,omitempty"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	NameEn        string  `json:"name_en,omitempty"`
	DescriptionEn string  `json:"description_en,omitempty"`
}

// CheckoutRechargePackage is the user-facing package card, including credited totals.
type CheckoutRechargePackage struct {
	ID            string  `json:"id"`
	Amount        float64 `json:"amount"`
	Bonus         float64 `json:"bonus"`
	Credit        float64 `json:"credit"`
	Badge         string  `json:"badge,omitempty"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	NameEn        string  `json:"name_en,omitempty"`
	DescriptionEn string  `json:"description_en,omitempty"`
}

// DefaultRechargePackages is the built-in card set used when no setting is stored.
func DefaultRechargePackages() []RechargePackage {
	return []RechargePackage{
		{ID: "starter", Amount: 50, Name: "体验", Description: "适合初次体验", NameEn: "Starter", DescriptionEn: "For a first try"},
		{ID: "standard", Amount: 100, Name: "标准", Description: "开发者常用", NameEn: "Standard", DescriptionEn: "Popular with developers"},
		{ID: "advanced", Amount: 500, Badge: rechargePackageBadgePop, Name: "进阶", Description: "进阶用户首选", NameEn: "Advanced", DescriptionEn: "For growing usage"},
		{ID: "pro", Amount: 1000, Badge: rechargePackageBadgeBest, Name: "专业", Description: "专业团队推荐", NameEn: "Pro", DescriptionEn: "For professional teams"},
		{ID: "team", Amount: 1500, Name: "团队", Description: "小团队协作", NameEn: "Team", DescriptionEn: "For small teams"},
		{ID: "business", Amount: 2000, Name: "商务", Description: "商务规模使用", NameEn: "Business", DescriptionEn: "For business workloads"},
		{ID: "premium", Amount: 3000, Name: "尊享", Description: "高频重度使用", NameEn: "Premium", DescriptionEn: "For heavy usage"},
		{ID: "enterprise", Amount: 5000, Name: "企业", Description: "企业级用量", NameEn: "Enterprise", DescriptionEn: "For enterprise volume"},
	}
}

func ParseRechargePackages(raw string) []RechargePackage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultRechargePackages()
	}
	var packages []RechargePackage
	if err := json.Unmarshal([]byte(raw), &packages); err != nil {
		return DefaultRechargePackages()
	}
	normalized, err := NormalizeRechargePackages(packages)
	if err != nil {
		return DefaultRechargePackages()
	}
	return normalized
}

func NormalizeRechargePackages(packages []RechargePackage) ([]RechargePackage, error) {
	if len(packages) == 0 {
		return DefaultRechargePackages(), nil
	}
	if len(packages) > maxRechargePackages {
		return nil, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("at most %d recharge packages are allowed", maxRechargePackages))
	}
	out := make([]RechargePackage, 0, len(packages))
	seenID := make(map[string]struct{}, len(packages))
	seenAmount := make(map[string]struct{}, len(packages))
	for i, pkg := range packages {
		normalized, err := normalizeRechargePackage(pkg, i)
		if err != nil {
			return nil, err
		}
		if _, ok := seenID[normalized.ID]; ok {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", "recharge package id must be unique")
		}
		amountKey := rechargeAmountKey(normalized.Amount)
		if _, ok := seenAmount[amountKey]; ok {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", "recharge package amount must be unique")
		}
		seenID[normalized.ID] = struct{}{}
		seenAmount[amountKey] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeRechargePackage(pkg RechargePackage, index int) (RechargePackage, error) {
	amount := decimal.NewFromFloat(pkg.Amount).Round(2)
	if !amount.IsPositive() {
		return RechargePackage{}, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("package #%d amount must be greater than 0", index+1))
	}
	bonus := decimal.NewFromFloat(pkg.Bonus).Round(2)
	if bonus.IsNegative() {
		return RechargePackage{}, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("package #%d bonus cannot be negative", index+1))
	}
	name := strings.TrimSpace(pkg.Name)
	if name == "" {
		return RechargePackage{}, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("package #%d name is required", index+1))
	}
	if len([]rune(name)) > maxRechargePackageName {
		return RechargePackage{}, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("package #%d name is too long", index+1))
	}
	description := strings.TrimSpace(pkg.Description)
	if len([]rune(description)) > maxRechargePackageDesc {
		return RechargePackage{}, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("package #%d description is too long", index+1))
	}
	nameEn := strings.TrimSpace(pkg.NameEn)
	descriptionEn := strings.TrimSpace(pkg.DescriptionEn)
	if len([]rune(nameEn)) > maxRechargePackageName {
		return RechargePackage{}, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("package #%d English name is too long", index+1))
	}
	if len([]rune(descriptionEn)) > maxRechargePackageDesc {
		return RechargePackage{}, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("package #%d English description is too long", index+1))
	}
	id := strings.TrimSpace(strings.ToLower(pkg.ID))
	if id == "" {
		id = defaultRechargePackageID(amount)
	}
	if !rechargePackageIDPattern.MatchString(id) {
		return RechargePackage{}, infraerrors.BadRequest("INVALID_RECHARGE_PACKAGES", fmt.Sprintf("package #%d id is invalid", index+1))
	}
	return RechargePackage{
		ID:            id,
		Amount:        amount.InexactFloat64(),
		Bonus:         bonus.InexactFloat64(),
		Badge:         normalizeRechargePackageBadge(pkg.Badge),
		Name:          name,
		Description:   description,
		NameEn:        nameEn,
		DescriptionEn: descriptionEn,
	}, nil
}

func defaultRechargePackageID(amount decimal.Decimal) string {
	cents := amount.Mul(decimal.NewFromInt(100)).Round(0).IntPart()
	return fmt.Sprintf("pkg-%d", cents)
}

func normalizeRechargePackageBadge(badge string) string {
	switch strings.TrimSpace(badge) {
	case rechargePackageBadgePop, rechargePackageBadgeBest:
		return strings.TrimSpace(badge)
	default:
		return ""
	}
}

func rechargeAmountKey(amount float64) string {
	return decimal.NewFromFloat(amount).Round(2).StringFixed(2)
}

func FindRechargePackageByAmount(packages []RechargePackage, amount float64) (RechargePackage, bool) {
	key := rechargeAmountKey(amount)
	for _, pkg := range packages {
		if rechargeAmountKey(pkg.Amount) == key {
			return pkg, true
		}
	}
	return RechargePackage{}, false
}

func BuildCheckoutRechargePackages(packages []RechargePackage, multiplier float64) []CheckoutRechargePackage {
	if len(packages) == 0 {
		packages = DefaultRechargePackages()
	}
	out := make([]CheckoutRechargePackage, 0, len(packages))
	for _, pkg := range packages {
		credit := calculateCreditedBalance(pkg.Amount, multiplier, packages)
		out = append(out, CheckoutRechargePackage{
			ID:            pkg.ID,
			Amount:        pkg.Amount,
			Bonus:         pkg.Bonus,
			Credit:        credit,
			Badge:         pkg.Badge,
			Name:          pkg.Name,
			Description:   pkg.Description,
			NameEn:        pkg.NameEn,
			DescriptionEn: pkg.DescriptionEn,
		})
	}
	return out
}
