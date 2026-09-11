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

func TestComparePlazaModelNames_DateSuffixAndTieBreak(t *testing.T) {
	require.Negative(t, comparePlazaModelNames("gpt-5.6-20251001", "gpt-5.6"))
	require.Negative(t, comparePlazaModelNames("gpt-5.6", "gpt-5.5"))
	require.Negative(t, comparePlazaModelNames("claude-opus", "claude-sonnet"))
}
