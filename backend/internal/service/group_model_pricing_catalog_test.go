package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
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

func TestAddEffectiveGroupAudioPricingUsesConfiguredAndDefaultPrices(t *testing.T) {
	realtime := 0.08
	freeTTS := 0.0
	group := &Group{
		Platform:                     PlatformGrok,
		AudioRealtimePricePerMin:     &realtime,
		AudioTTSPricePerMillionChars: &freeTTS,
	}
	catalog := BuildEffectiveGroupModelPricingCatalog(group, 0.5)

	AddEffectiveGroupAudioPricing(&catalog, group, 0.5)

	require.Equal(t, 0.5, catalog.TokenMultiplier)
	require.Equal(t, GroupAudioPricingUnitMinute, catalog.Audio[GroupAudioPricingGrokRealtime].Unit)
	require.Equal(t, GroupAudioPricingSourceGroup, catalog.Audio[GroupAudioPricingGrokRealtime].Source)
	require.Equal(t, 0.04, catalog.Audio[GroupAudioPricingGrokRealtime].Prices["default"])
	require.Equal(t, GroupAudioPricingUnitMillionCharacters, catalog.Audio[GroupAudioPricingTTS].Unit)
	require.Equal(t, GroupAudioPricingSourceGroup, catalog.Audio[GroupAudioPricingTTS].Source)
	require.Zero(t, catalog.Audio[GroupAudioPricingTTS].Prices["default"])
	require.Equal(t, GroupAudioPricingUnitHour, catalog.Audio[GroupAudioPricingSTT].Unit)
	require.Equal(t, GroupAudioPricingSourceDefault, catalog.Audio[GroupAudioPricingSTT].Source)
	require.Equal(t, 0.05, catalog.Audio[GroupAudioPricingSTT].Prices["default"])
}

func TestAddEffectiveGroupAudioPricingDoesNotChangeTokenMultiplier(t *testing.T) {
	group := &Group{Platform: PlatformGrok}
	catalog := BuildEffectiveGroupModelPricingCatalog(group, 0.5)
	catalog.TokenMultiplier = 1.25

	AddEffectiveGroupAudioPricing(&catalog, group, 0.5)

	require.Equal(t, 1.25, catalog.TokenMultiplier)
	require.Equal(t, defaultAudioRealtimePricePerMin*0.5, catalog.Audio[GroupAudioPricingGrokRealtime].Prices["default"])
}

func TestAddEffectiveGroupAudioPricingSkipsUnsupportedPlatforms(t *testing.T) {
	group := &Group{Platform: PlatformOpenAI}
	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)

	AddEffectiveGroupAudioPricing(&catalog, group, 1)

	require.Empty(t, catalog.Audio)
}

func TestAddEffectiveGroupAudioPricingLimitsCompositeToEnabledRealtime(t *testing.T) {
	disabled := &Group{Platform: PlatformComposite}
	disabledCatalog := BuildEffectiveGroupModelPricingCatalog(disabled, 1)
	AddEffectiveGroupAudioPricing(&disabledCatalog, disabled, 1)
	require.Empty(t, disabledCatalog.Audio)

	enabled := &Group{Platform: PlatformComposite, AllowRealtime: true}
	enabledCatalog := BuildEffectiveGroupModelPricingCatalog(enabled, 1)
	AddEffectiveGroupAudioPricing(&enabledCatalog, enabled, 1)
	require.Contains(t, enabledCatalog.Audio, GroupAudioPricingGrokRealtime)
	require.NotContains(t, enabledCatalog.Audio, GroupAudioPricingTTS)
	require.NotContains(t, enabledCatalog.Audio, GroupAudioPricingSTT)
}

func TestAddEffectiveAudioTokenModelPricingUsesResolvedBillingPrices(t *testing.T) {
	pricingService := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-realtime-test": {
			InputCostPerToken:                2e-6,
			OutputCostPerToken:               4e-6,
			CacheReadInputTokenCost:          1e-7,
			InputCostPerAudioToken:           32e-6,
			OutputCostPerAudioToken:          64e-6,
			CacheReadInputAudioTokenCost:     0.4e-6,
			CacheCreationInputAudioTokenCost: 0.4e-6,
		},
	}}
	billing := NewBillingService(&config.Config{}, pricingService)
	resolver := NewModelPricingResolver(nil, billing)
	group := &Group{ID: 71, Platform: PlatformOpenAI, AllowRealtime: true}
	catalog := BuildEffectiveGroupModelPricingCatalog(group, 0.5)

	added := resolver.AddEffectiveAudioTokenModelPricing(
		context.Background(), &catalog, group, "gpt-realtime-test", 0.5, time.Now(),
	)

	require.True(t, added)
	model := catalog.Models["gpt-realtime-test"]
	require.True(t, model.Displayable)
	require.Equal(t, BillingModeToken, model.BillingMode)
	require.Equal(t, GroupModelPricingUnitToken, model.Unit)
	require.Equal(t, 16e-6, model.Prices[GroupModelPricingAudioInputPriceKey])
	require.Equal(t, 32e-6, model.Prices[GroupModelPricingAudioOutputPriceKey])
	require.Equal(t, 0.2e-6, model.Prices[GroupModelPricingAudioCacheReadPriceKey])
}

func TestAddEffectiveAudioTokenModelPricingUsesEmbeddedGPTRealtime21Rates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	pricingService := &PricingService{}
	pricingData, err := pricingService.parsePricingData(data)
	require.NoError(t, err)
	pricingService.pricingData = pricingData
	billing := NewBillingService(&config.Config{}, pricingService)
	resolver := NewModelPricingResolver(nil, billing)
	group := &Group{ID: 71, Platform: PlatformOpenAI, AllowRealtime: true}
	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)

	added := resolver.AddEffectiveAudioTokenModelPricing(
		context.Background(), &catalog, group, "gpt-realtime-2.1", 1, time.Now(),
	)

	require.True(t, added)
	model := catalog.Models["gpt-realtime-2.1"]
	require.InDelta(t, 32e-6, model.Prices[GroupModelPricingAudioInputPriceKey], 1e-12)
	require.InDelta(t, 64e-6, model.Prices[GroupModelPricingAudioOutputPriceKey], 1e-12)
	require.InDelta(t, 0.4e-6, model.Prices[GroupModelPricingAudioCacheReadPriceKey], 1e-12)
}

func TestAddEffectiveAudioTokenModelPricingRejectsPerRequestAndTextOnlyModels(t *testing.T) {
	price := 0.1
	group := &Group{ID: 71, Platform: PlatformOpenAI, AllowRealtime: true, ModelPricing: []ChannelModelPricing{{
		Models:          []string{"per-request-realtime"},
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: &price,
	}}}
	pricingService := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"text-realtime": {
			InputCostPerToken:  1e-6,
			OutputCostPerToken: 2e-6,
		},
		"partial-audio-realtime": {
			InputCostPerToken:       1e-6,
			OutputCostPerToken:      2e-6,
			InputCostPerAudioToken:  32e-6,
			OutputCostPerAudioToken: 64e-6,
		},
	}}
	billing := NewBillingService(&config.Config{}, pricingService)
	resolver := NewModelPricingResolver(nil, billing)
	catalog := BuildEffectiveGroupModelPricingCatalog(group, 1)

	require.False(t, resolver.AddEffectiveAudioTokenModelPricing(
		context.Background(), &catalog, group, "per-request-realtime", 1, time.Now(),
	))
	require.False(t, resolver.AddEffectiveAudioTokenModelPricing(
		context.Background(), &catalog, group, "text-realtime", 1, time.Now(),
	))
	require.False(t, resolver.AddEffectiveAudioTokenModelPricing(
		context.Background(), &catalog, group, "partial-audio-realtime", 1, time.Now(),
	))

	disabled := &Group{ID: 72, Platform: PlatformOpenAI}
	require.False(t, resolver.AddEffectiveAudioTokenModelPricing(
		context.Background(), &catalog, disabled, "gpt-realtime-test", 1, time.Now(),
	))
}
