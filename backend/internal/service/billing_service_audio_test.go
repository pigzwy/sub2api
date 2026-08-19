package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// gpt-realtime 官方价：text in $4/M、audio in $32/M、cached $0.4/M、
// text out $16/M、audio out $64/M。
func realtimeAudioTestPricing() *ModelPricing {
	return &ModelPricing{
		InputPricePerToken:          4e-6,
		OutputPricePerToken:         1.6e-5,
		CacheReadPricePerToken:      4e-7,
		AudioInputPricePerToken:     3.2e-5,
		AudioOutputPricePerToken:    6.4e-5,
		AudioCacheReadPricePerToken: 4e-7,
	}
}

func TestComputeTokenBreakdownAudioTokens(t *testing.T) {
	s := &BillingService{}
	// RecordUsage 口径：InputTokens 为去除缓存后的互斥输入桶，
	// AudioInputTokens 是其中的音频子集；CacheReadTokens 含音频缓存子集。
	tokens := UsageTokens{
		InputTokens:          800, // 300 text + 500 audio（非缓存）
		AudioInputTokens:     500,
		OutputTokens:         500, // 120 text + 380 audio
		AudioOutputTokens:    380,
		CacheReadTokens:      200, // 100 text + 100 audio
		AudioCacheReadTokens: 100,
	}
	bd := s.computeTokenBreakdown(realtimeAudioTestPricing(), tokens, 1.0, "", false)

	require.InEpsilon(t, 300*4e-6, bd.InputCost, 1e-9)
	require.InEpsilon(t, 500*3.2e-5, bd.AudioInputCost, 1e-9)
	require.InEpsilon(t, 120*1.6e-5, bd.OutputCost, 1e-9)
	require.InEpsilon(t, 380*6.4e-5, bd.AudioOutputCost, 1e-9)
	require.InEpsilon(t, 100*4e-7+100*4e-7, bd.CacheReadCost, 1e-9)
	expectedTotal := bd.InputCost + bd.AudioInputCost + bd.OutputCost + bd.AudioOutputCost + bd.CacheReadCost
	require.InEpsilon(t, expectedTotal, bd.TotalCost, 1e-9)
	require.InEpsilon(t, expectedTotal, bd.ActualCost, 1e-9)
}

func TestComputeTokenBreakdownAudioAppliesRateMultiplier(t *testing.T) {
	s := &BillingService{}
	tokens := UsageTokens{
		InputTokens:       100,
		AudioInputTokens:  100,
		OutputTokens:      100,
		AudioOutputTokens: 100,
	}
	bd := s.computeTokenBreakdown(realtimeAudioTestPricing(), tokens, 2.0, "", false)
	require.InEpsilon(t, bd.TotalCost*2.0, bd.ActualCost, 1e-9)
}

func TestComputeTokenBreakdownAudioFallsBackToTextPrices(t *testing.T) {
	s := &BillingService{}
	// 未配置音频价的模型：audio token 回退按文本价计，不产生 $0 计费。
	pricing := &ModelPricing{
		InputPricePerToken:     1e-6,
		OutputPricePerToken:    2e-6,
		CacheReadPricePerToken: 1e-7,
	}
	tokens := UsageTokens{
		InputTokens:          100,
		AudioInputTokens:     40,
		OutputTokens:         50,
		AudioOutputTokens:    50,
		CacheReadTokens:      30,
		AudioCacheReadTokens: 30,
	}
	bd := s.computeTokenBreakdown(pricing, tokens, 1.0, "", false)
	require.InEpsilon(t, 60*1e-6, bd.InputCost, 1e-9)
	require.InEpsilon(t, 40*1e-6, bd.AudioInputCost, 1e-9)
	require.Zero(t, bd.OutputCost)
	require.InEpsilon(t, 50*2e-6, bd.AudioOutputCost, 1e-9)
	require.InEpsilon(t, 30*1e-7, bd.CacheReadCost, 1e-9)
}

func TestComputeTokenBreakdownWithoutAudioUnchanged(t *testing.T) {
	s := &BillingService{}
	pricing := &ModelPricing{
		InputPricePerToken:  1e-6,
		OutputPricePerToken: 2e-6,
	}
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500}
	bd := s.computeTokenBreakdown(pricing, tokens, 1.0, "", false)
	require.InEpsilon(t, 1000*1e-6, bd.InputCost, 1e-9)
	require.InEpsilon(t, 500*2e-6, bd.OutputCost, 1e-9)
	require.Zero(t, bd.AudioInputCost)
	require.Zero(t, bd.AudioOutputCost)
	require.InEpsilon(t, 1000*1e-6+500*2e-6, bd.TotalCost, 1e-9)
}
