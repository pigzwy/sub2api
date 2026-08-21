package service

import (
	"strings"
)

const (
	GroupModelPricingMatchExact  = "exact"
	GroupModelPricingMatchPrefix = "prefix"
	GroupModelPricingUnitImage   = "image"
	GroupModelPricingUnitSecond  = "second"
)

// EffectiveGroupModelPrice is one explicitly configured group model rule.
// Displayable media rules have the same multiplier applied as usage billing.
type EffectiveGroupModelPrice struct {
	MatchType   string
	Priority    *int
	BillingMode BillingMode
	Displayable bool
	Unit        string
	Prices      map[string]float64
}

// EffectiveGroupModelPricingCatalog contains every explicit group-level model
// rule. Image and video rules include directly displayable effective prices;
// other modes remain visible as non-displayable precedence blockers. Channel
// and built-in fallback prices are deliberately excluded.
type EffectiveGroupModelPricingCatalog struct {
	ResolvedGroupMultiplier float64
	ImageMultiplier         float64
	VideoMultiplier         float64
	Models                  map[string]EffectiveGroupModelPrice
}

func BuildEffectiveGroupModelPricingCatalog(group *Group, resolvedGroupMultiplier float64) EffectiveGroupModelPricingCatalog {
	if resolvedGroupMultiplier < 0 {
		resolvedGroupMultiplier = 0
	}
	apiKey := &APIKey{Group: group}
	catalog := EffectiveGroupModelPricingCatalog{
		ResolvedGroupMultiplier: resolvedGroupMultiplier,
		ImageMultiplier:         resolveImageRateMultiplier(apiKey, resolvedGroupMultiplier),
		VideoMultiplier:         resolveVideoRateMultiplier(apiKey, resolvedGroupMultiplier),
		Models:                  make(map[string]EffectiveGroupModelPrice),
	}
	if group == nil {
		return catalog
	}

	seen := make(map[string]struct{})
	prefixPriority := 0
	for i := range group.ModelPricing {
		entry := &group.ModelPricing[i]
		var tiers []string
		var unit string
		var multiplier float64
		billingMode := entry.BillingMode
		if billingMode == "" {
			billingMode = BillingModeToken
		}
		displayable := true
		switch billingMode {
		case BillingModeImage:
			tiers = []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K}
			unit = GroupModelPricingUnitImage
			multiplier = catalog.ImageMultiplier
		case BillingModeVideo:
			tiers = []string{VideoBillingResolution480P, VideoBillingResolution720P, VideoBillingResolution1080P}
			unit = GroupModelPricingUnitSecond
			multiplier = catalog.VideoMultiplier
		default:
			displayable = false
		}

		var prices map[string]float64
		if displayable {
			prices = make(map[string]float64, len(tiers))
			for _, tier := range tiers {
				prices[tier] = configuredMediaTierPrice(entry, tier) * multiplier
			}
		}
		for _, configuredModel := range entry.Models {
			model := normalizeChannelPricingModelName(configuredModel)
			if model == "" {
				continue
			}
			if _, exists := seen[model]; exists {
				continue
			}
			seen[model] = struct{}{}
			matchType := GroupModelPricingMatchExact
			var priority *int
			if strings.HasSuffix(model, "*") {
				matchType = GroupModelPricingMatchPrefix
				value := prefixPriority
				priority = &value
				prefixPriority++
			}
			catalog.Models[model] = EffectiveGroupModelPrice{
				MatchType:   matchType,
				Priority:    priority,
				BillingMode: billingMode,
				Displayable: displayable,
				Unit:        unit,
				Prices:      cloneGroupModelPricingPrices(prices),
			}
		}
	}
	return catalog
}

// configuredMediaTierPrice mirrors calculatePerRequestCost for media requests:
// an exact non-zero tier wins, followed by the zero-context interval, then the
// default per-request price.
func configuredMediaTierPrice(entry *ChannelModelPricing, tierLabel string) float64 {
	if entry == nil {
		return 0
	}
	validTiers := filterValidIntervals(entry.Intervals)
	for _, tier := range validTiers {
		if tier.TierLabel == tierLabel && tier.PerRequestPrice != nil {
			if *tier.PerRequestPrice != 0 {
				return *tier.PerRequestPrice
			}
			break
		}
	}
	if interval := FindMatchingInterval(validTiers, 0); interval != nil && interval.PerRequestPrice != nil {
		if *interval.PerRequestPrice != 0 {
			return *interval.PerRequestPrice
		}
	}
	if entry.PerRequestPrice != nil {
		return *entry.PerRequestPrice
	}
	return 0
}

func cloneGroupModelPricingPrices(input map[string]float64) map[string]float64 {
	if input == nil {
		return nil
	}
	out := make(map[string]float64, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
