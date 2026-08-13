package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The admin settings round-trip parses menu items into this struct and
// re-serializes them, so a field missing here is silently dropped on save.
// These tests pin open_mode to that contract.

func TestParseCustomMenuItems_PreservesOpenMode(t *testing.T) {
	raw := `[{"id":"a","label":"A","icon_svg":"","url":"https://example.test/a","visibility":"user","sort_order":0,"open_mode":"blank"}]`

	items := ParseCustomMenuItems(raw)
	require.Len(t, items, 1)
	require.Equal(t, "blank", items[0].OpenMode)

	// Re-serializing must keep the value: this is the admin save path.
	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"open_mode":"blank"`)
}

// Items stored before the option existed have no open_mode; they must survive
// a round-trip unchanged and stay on the iframe default rather than gaining a
// spurious value.
func TestParseCustomMenuItems_LegacyItemHasEmptyOpenMode(t *testing.T) {
	raw := `[{"id":"a","label":"A","icon_svg":"","url":"https://example.test/a","visibility":"user","sort_order":0}]`

	items := ParseCustomMenuItems(raw)
	require.Len(t, items, 1)
	require.Equal(t, "", items[0].OpenMode)

	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "open_mode")
}

func TestParseUserVisibleMenuItems_PreservesOpenMode(t *testing.T) {
	raw := `[
		{"id":"a","label":"A","icon_svg":"","url":"https://example.test/a","visibility":"user","sort_order":0,"open_mode":"self"},
		{"id":"b","label":"B","icon_svg":"","url":"https://example.test/b","visibility":"admin","sort_order":1,"open_mode":"blank"}
	]`

	items := ParseUserVisibleMenuItems(raw)
	require.Len(t, items, 1, "admin-only item must not reach the public settings payload")
	require.Equal(t, "a", items[0].ID)
	require.Equal(t, "self", items[0].OpenMode)
}
