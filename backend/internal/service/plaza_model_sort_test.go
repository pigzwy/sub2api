//go:build unit

package service

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComparePlazaModelNames_NewestVersionFirst(t *testing.T) {
	names := []string{
		"claude-haiku-4-5",
		"claude-opus-4-6",
		"claude-opus-4-8",
		"claude-fable-5-1",
	}
	sort.SliceStable(names, func(i, j int) bool {
		return comparePlazaModelNames(names[i], names[j]) < 0
	})
	require.Equal(t, []string{
		"claude-fable-5-1",
		"claude-opus-4-8",
		"claude-opus-4-6",
		"claude-haiku-4-5",
	}, names)
}

func TestComparePlazaModelNames_OpenAIAndGrokNewestFirst(t *testing.T) {
	openai := []string{"gpt-5", "gpt-5.5", "gpt-5.6-sol"}
	sort.SliceStable(openai, func(i, j int) bool {
		return comparePlazaModelNames(openai[i], openai[j]) < 0
	})
	require.Equal(t, []string{"gpt-5.6-sol", "gpt-5.5", "gpt-5"}, openai)

	grok := []string{"grok-4.3", "grok-4.6", "grok-4.5"}
	sort.SliceStable(grok, func(i, j int) bool {
		return comparePlazaModelNames(grok[i], grok[j]) < 0
	})
	require.Equal(t, []string{"grok-4.6", "grok-4.5", "grok-4.3"}, grok)
}

func TestComparePlazaModelNames_DateSuffixAndTieBreak(t *testing.T) {
	require.Negative(t, comparePlazaModelNames("gpt-5.6-20251001", "gpt-5.6"))
	require.Negative(t, comparePlazaModelNames("gpt-5.6", "gpt-5.5"))
	require.Negative(t, comparePlazaModelNames("claude-opus", "claude-sonnet"))
}
