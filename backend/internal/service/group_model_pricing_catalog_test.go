package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func catalogPrice(value float64) *float64 { return &value }

func catalogTiers(labels []string, prices []float64) []PricingInterval {
	out := make([]PricingInterval, 0, len(labels))
	for i := range labels {
		out = append(out, PricingInterval{TierLabel: labels[i], PerRequestPrice: catalogPrice(prices[i])})
	}
	return out
}

func TestBuildEffectiveGroupModelPricingCatalogMatchesConfiguredMediaTable(t *testing.T) {
	group := &Group{
		ID:             71,
		RateMultiplier: 1,
		ModelPricing: []ChannelModelPricing{
			{
				Models:      []string{"gpt-image-2", "grok-imagine-image", "grok-imagine-image-quality"},
				BillingMode: BillingModeImage,
				Intervals: catalogTiers(
					[]string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K},
					[]float64{0.1, 0.2, 0.3},
				),
			},
			{
				Models:      []string{"gemini-3-pro-image", "grok-imagine-image-2.0"},
				BillingMode: BillingModeImage,
				Intervals: catalogTiers(
					[]string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K},
					[]float64{0.2, 0.3, 0.4},
				),
			},
			{
				Models:      []string{"grok-imagine-video"},
				BillingMode: BillingModeVideo,
				Intervals: catalogTiers(
					[]string{VideoBillingResolution480P, VideoBillingResolution720P, VideoBillingResolution1080P},
					[]float64{0.03, 0.04, 0.05},
				),
			},
			{
				Models:      []string{"grok-imagine-video-1.5"},
				BillingMode: BillingModeVideo,
				Intervals: catalogTiers(
					[]string{VideoBillingResolution480P, VideoBillingResolution720P, VideoBillingResolution1080P},
					[]float64{0.05, 0.08, 0.1},
				),
			},
		},
	}

	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)

	require.Equal(t, 1.0, catalog.ResolvedGroupMultiplier)
	require.Equal(t, 1.0, catalog.ImageMultiplier)
	require.Equal(t, 1.0, catalog.VideoMultiplier)
	require.Len(t, catalog.Models, 7)
	require.Equal(t, map[string]float64{"1K": 0.1, "2K": 0.2, "4K": 0.3}, catalog.Models["gpt-image-2"].Prices)
	require.Equal(t, map[string]float64{"1K": 0.1, "2K": 0.2, "4K": 0.3}, catalog.Models["grok-imagine-image"].Prices)
	require.Equal(t, map[string]float64{"1K": 0.1, "2K": 0.2, "4K": 0.3}, catalog.Models["grok-imagine-image-quality"].Prices)
	require.Equal(t, map[string]float64{"1K": 0.2, "2K": 0.3, "4K": 0.4}, catalog.Models["gemini-3-pro-image"].Prices)
	require.Equal(t, map[string]float64{"1K": 0.2, "2K": 0.3, "4K": 0.4}, catalog.Models["grok-imagine-image-2.0"].Prices)
	require.Equal(t, map[string]float64{"480p": 0.03, "720p": 0.04, "1080p": 0.05}, catalog.Models["grok-imagine-video"].Prices)
	require.Equal(t, map[string]float64{"480p": 0.05, "720p": 0.08, "1080p": 0.1}, catalog.Models["grok-imagine-video-1.5"].Prices)
	require.True(t, catalog.Models["gpt-image-2"].Displayable)
	require.Equal(t, GroupModelPricingUnitImage, catalog.Models["gpt-image-2"].Unit)
	require.Equal(t, GroupModelPricingUnitSecond, catalog.Models["grok-imagine-video"].Unit)
}

func TestBuildEffectiveGroupModelPricingCatalogAppliesBillingMultipliers(t *testing.T) {
	group := &Group{
		ID:                   71,
		RateMultiplier:       0.75,
		ImageRateIndependent: true,
		ImageRateMultiplier:  2,
		VideoRateIndependent: false,
		VideoRateMultiplier:  9,
		ModelPricing: []ChannelModelPricing{
			{
				Models:          []string{"IMAGE-*"},
				BillingMode:     BillingModeImage,
				PerRequestPrice: catalogPrice(0.1),
				Intervals: []PricingInterval{
					{TierLabel: ImageBillingSize2K, PerRequestPrice: catalogPrice(0)},
				},
			},
			{
				Models:          []string{"video-model"},
				BillingMode:     BillingModeVideo,
				PerRequestPrice: catalogPrice(0.04),
			},
			{Models: []string{"text-model"}, BillingMode: BillingModeToken},
		},
	}

	catalog := BuildEffectiveGroupModelPricingCatalog(group, 0.5)

	require.Equal(t, 0.5, catalog.ResolvedGroupMultiplier)
	require.Equal(t, 2.0, catalog.ImageMultiplier)
	require.Equal(t, 0.5, catalog.VideoMultiplier)
	require.Len(t, catalog.Models, 3)
	require.Equal(t, GroupModelPricingMatchPrefix, catalog.Models["image-*"].MatchType)
	require.Equal(t, map[string]float64{"1K": 0.2, "2K": 0.2, "4K": 0.2}, catalog.Models["image-*"].Prices)
	require.Equal(t, map[string]float64{"480p": 0.02, "720p": 0.02, "1080p": 0.02}, catalog.Models["video-model"].Prices)
	require.False(t, catalog.Models["text-model"].Displayable)
	require.Nil(t, catalog.Models["text-model"].Prices)
	require.Empty(t, catalog.Models["text-model"].Unit)
}

func TestBuildEffectiveGroupModelPricingCatalogKeepsFirstDuplicateAndEmptyMap(t *testing.T) {
	first := 0.1
	second := 0.9
	group := &Group{ModelPricing: []ChannelModelPricing{
		{Models: []string{"same-model"}, BillingMode: BillingModeImage, PerRequestPrice: &first},
		{Models: []string{"SAME-MODEL"}, BillingMode: BillingModeImage, PerRequestPrice: &second},
	}}

	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)
	require.Equal(t, 0.1, catalog.Models["same-model"].Prices[ImageBillingSize2K])

	empty := BuildEffectiveGroupModelPricingCatalog(&Group{}, 1)
	require.NotNil(t, empty.Models)
	require.Empty(t, empty.Models)
}

func TestBuildEffectiveGroupModelPricingCatalogExposesPrefixPriority(t *testing.T) {
	price := 0.1
	group := &Group{ModelPricing: []ChannelModelPricing{
		{Models: []string{"grok-*"}, BillingMode: BillingModeToken},
		{Models: []string{"exact-model"}, BillingMode: BillingModeImage, PerRequestPrice: &price},
		{Models: []string{"grok-imagine-*", "GROK-*"}, BillingMode: BillingModeImage, PerRequestPrice: &price},
	}}

	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)
	require.NotNil(t, catalog.Models["grok-*"].Priority)
	require.Equal(t, 0, *catalog.Models["grok-*"].Priority)
	require.False(t, catalog.Models["grok-*"].Displayable)
	require.NotNil(t, catalog.Models["grok-imagine-*"].Priority)
	require.Equal(t, 1, *catalog.Models["grok-imagine-*"].Priority)
	require.True(t, catalog.Models["grok-imagine-*"].Displayable)
	require.Nil(t, catalog.Models["exact-model"].Priority)
}

func TestBuildEffectiveGroupModelPricingCatalogPreservesExactOverWildcard(t *testing.T) {
	wildcardPrice := 0.9
	exactPrice := 0.2
	group := &Group{ModelPricing: []ChannelModelPricing{
		{Models: []string{"grok-*"}, BillingMode: BillingModeImage, PerRequestPrice: &wildcardPrice},
		{Models: []string{"grok-image"}, BillingMode: BillingModeImage, PerRequestPrice: &exactPrice},
	}}

	matched := matchGroupModelPricing(group, "grok-image")
	require.NotNil(t, matched)
	require.Equal(t, exactPrice, *matched.PerRequestPrice)

	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)
	require.Equal(t, GroupModelPricingMatchPrefix, catalog.Models["grok-*"].MatchType)
	require.Equal(t, 0, *catalog.Models["grok-*"].Priority)
	require.Equal(t, GroupModelPricingMatchExact, catalog.Models["grok-image"].MatchType)
	require.Nil(t, catalog.Models["grok-image"].Priority)
	require.Equal(t, exactPrice, catalog.Models["grok-image"].Prices[ImageBillingSize2K])
}

func TestBuildEffectiveGroupModelPricingCatalogMarksPerRequestAsNonDisplayable(t *testing.T) {
	price := 0.2
	group := &Group{ModelPricing: []ChannelModelPricing{{
		Models:          []string{"legacy-image-model"},
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: &price,
	}}}

	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)
	model := catalog.Models["legacy-image-model"]
	require.Equal(t, BillingModePerRequest, model.BillingMode)
	require.False(t, model.Displayable)
	require.Nil(t, model.Prices)
}

func TestBuildEffectiveGroupModelPricingCatalogMirrorsTierFallbackAndClampsMultiplier(t *testing.T) {
	defaultPrice := 0.1
	contextPrice := 0.7
	explicitZero := 0.0
	group := &Group{ModelPricing: []ChannelModelPricing{{
		Models:          []string{"image-model"},
		BillingMode:     BillingModeImage,
		PerRequestPrice: &defaultPrice,
		Intervals: []PricingInterval{
			{MinTokens: -1, TierLabel: ImageBillingSize1K, PerRequestPrice: &contextPrice},
			{TierLabel: ImageBillingSize2K, PerRequestPrice: &explicitZero},
		},
	}}}

	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)
	require.Equal(t, 0.7, catalog.Models["image-model"].Prices[ImageBillingSize2K])

	clamped := BuildEffectiveGroupModelPricingCatalog(group, -2)
	require.Zero(t, clamped.ResolvedGroupMultiplier)
	require.Zero(t, clamped.ImageMultiplier)
	require.Zero(t, clamped.Models["image-model"].Prices[ImageBillingSize2K])
}
