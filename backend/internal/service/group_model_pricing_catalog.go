package service

import (
	"context"
	"strings"
	"time"
)

const (
	GroupModelPricingMatchExact  = "exact"
	GroupModelPricingMatchPrefix = "prefix"
	GroupModelPricingUnitImage   = "image"
	GroupModelPricingUnitSecond  = "second"
	GroupModelPricingUnitToken   = "token"

	GroupModelPricingAudioInputPriceKey     = "audio_input"
	GroupModelPricingAudioOutputPriceKey    = "audio_output"
	GroupModelPricingAudioCacheReadPriceKey = "audio_cache_read"

	GroupAudioPricingGrokRealtime = "grok_realtime"
	GroupAudioPricingTTS          = "tts"
	GroupAudioPricingSTT          = "stt"

	GroupAudioPricingUnitMinute            = "minute"
	GroupAudioPricingUnitMillionCharacters = "million_characters"
	GroupAudioPricingUnitHour              = "hour"

	GroupAudioPricingSourceGroup   = "group"
	GroupAudioPricingSourceDefault = "default"
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

// EffectiveGroupAudioPrice is a group-wide audio fallback price. Unlike
// OpenAI Realtime token prices, these modes are not selected by model name.
type EffectiveGroupAudioPrice struct {
	Unit   string
	Source string
	Prices map[string]float64
}

// EffectiveGroupModelPricingCatalog starts with every explicit group-level
// model rule. Image and video rules include directly displayable effective
// prices; other modes remain visible as non-displayable precedence blockers.
// Callers may then add resolved audio-token prices through the resolver without
// exposing which internal pricing source supplied the final billed amount.
type EffectiveGroupModelPricingCatalog struct {
	ResolvedGroupMultiplier float64
	TokenMultiplier         float64
	ImageMultiplier         float64
	VideoMultiplier         float64
	Models                  map[string]EffectiveGroupModelPrice
	Audio                   map[string]EffectiveGroupAudioPrice
}

func BuildEffectiveGroupModelPricingCatalog(group *Group, resolvedGroupMultiplier float64) EffectiveGroupModelPricingCatalog {
	if resolvedGroupMultiplier < 0 {
		resolvedGroupMultiplier = 0
	}
	apiKey := &APIKey{Group: group}
	catalog := EffectiveGroupModelPricingCatalog{
		ResolvedGroupMultiplier: resolvedGroupMultiplier,
		TokenMultiplier:         resolvedGroupMultiplier,
		ImageMultiplier:         resolveImageRateMultiplier(apiKey, resolvedGroupMultiplier),
		VideoMultiplier:         resolveVideoRateMultiplier(apiKey, resolvedGroupMultiplier),
		Models:                  make(map[string]EffectiveGroupModelPrice),
		Audio:                   make(map[string]EffectiveGroupAudioPrice),
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

// AddEffectiveGroupAudioPricing adds group-wide audio modes that the platform
// can currently serve.
// rateMultiplier is the resolved user/group multiplier without the token-only
// peak factor, matching CalculateAudioCost in the OpenAI/Grok usage path.
func AddEffectiveGroupAudioPricing(catalog *EffectiveGroupModelPricingCatalog, group *Group, rateMultiplier float64) {
	if catalog == nil || group == nil {
		return
	}
	if group.Platform != PlatformGrok && (group.Platform != PlatformComposite || !group.AllowRealtime) {
		return
	}
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}
	catalog.Audio[GroupAudioPricingGrokRealtime] = effectiveGroupAudioPrice(
		group.AudioRealtimePricePerMin,
		defaultAudioRealtimePricePerMin,
		GroupAudioPricingUnitMinute,
		rateMultiplier,
	)
	// Composite currently exposes only the model-routed Realtime endpoint.
	// TTS/STT still fail closed inside GrokVoice until their endpoint routing is
	// explicitly defined, so do not advertise prices for unusable capabilities.
	if group.Platform == PlatformComposite {
		return
	}
	catalog.Audio[GroupAudioPricingTTS] = effectiveGroupAudioPrice(
		group.AudioTTSPricePerMillionChars,
		defaultAudioTTSPricePerMillionChars,
		GroupAudioPricingUnitMillionCharacters,
		rateMultiplier,
	)
	catalog.Audio[GroupAudioPricingSTT] = effectiveGroupAudioPrice(
		group.AudioSTTPricePerHour,
		defaultAudioSTTPricePerHour,
		GroupAudioPricingUnitHour,
		rateMultiplier,
	)
}

func effectiveGroupAudioPrice(configured *float64, fallback float64, unit string, multiplier float64) EffectiveGroupAudioPrice {
	price := fallback
	source := GroupAudioPricingSourceDefault
	if configured != nil {
		price = *configured
		source = GroupAudioPricingSourceGroup
	}
	if price < 0 {
		price = 0
	}
	return EffectiveGroupAudioPrice{
		Unit:   unit,
		Source: source,
		Prices: map[string]float64{"default": price * multiplier},
	}
}

// AddEffectiveAudioTokenModelPricing resolves the same model price used by
// token billing and adds an exact, directly displayable catalog entry. It is
// intentionally limited to models with explicit audio-token pricing so normal
// text models are not misrepresented as Realtime models.
func (r *ModelPricingResolver) AddEffectiveAudioTokenModelPricing(
	ctx context.Context,
	catalog *EffectiveGroupModelPricingCatalog,
	group *Group,
	model string,
	tokenMultiplier float64,
	pricingAt time.Time,
) bool {
	if r == nil || catalog == nil || group == nil {
		return false
	}
	if !group.AllowRealtime || (group.Platform != PlatformOpenAI && group.Platform != PlatformComposite) {
		return false
	}
	model = normalizeChannelPricingModelName(model)
	if model == "" || strings.HasSuffix(model, "*") {
		return false
	}
	groupID := group.ID
	resolved := r.Resolve(ctx, PricingInput{Model: model, GroupID: &groupID, Group: group})
	if resolved == nil || resolved.Mode != BillingModeToken {
		return false
	}
	pricing := resolved.BasePricing
	// Billing falls back to context-dependent text rates for any missing audio
	// bucket. A static catalog cannot represent that safely, so expose the model
	// only when all three dedicated Realtime rates are independently known.
	if pricing == nil || pricing.AudioInputPricePerToken <= 0 || pricing.AudioOutputPricePerToken <= 0 || pricing.AudioCacheReadPricePerToken <= 0 {
		return false
	}
	if tokenMultiplier < 0 {
		tokenMultiplier = 0
	}
	tokenMultiplier *= resolvedChannelTimeMultiplier(resolved, pricingAt)
	catalog.Models[model] = EffectiveGroupModelPrice{
		MatchType:   GroupModelPricingMatchExact,
		BillingMode: BillingModeToken,
		Displayable: true,
		Unit:        GroupModelPricingUnitToken,
		Prices: map[string]float64{
			GroupModelPricingAudioInputPriceKey:     pricing.AudioInputPricePerToken * tokenMultiplier,
			GroupModelPricingAudioOutputPriceKey:    pricing.AudioOutputPricePerToken * tokenMultiplier,
			GroupModelPricingAudioCacheReadPriceKey: pricing.AudioCacheReadPricePerToken * tokenMultiplier,
		},
	}
	return true
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
