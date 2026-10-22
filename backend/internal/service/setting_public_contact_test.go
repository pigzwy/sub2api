//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingService_CompactContactImagesPreserveRawSettings(t *testing.T) {
	imageBytes := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, (1<<20)-8)...)
	image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	card := fmt.Sprintf(`{"title":"QR","image":%q,"caption":"Scan"}`, image)
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			raw := fmt.Sprintf(`{"version":1,"enabled":%t,"extra":9007199254740993,"tabs":[{"id":"service","email":"support@example.com","cards":[%s,%s]},{"id":"support","cards":[%s,%s]},{"id":"business","cards":[%s,%s]}]}`,
				enabled, card, card, card, card, card, card)
			repo := &settingPublicRepoStub{values: map[string]string{SettingKeyContactInfo: raw}}
			svc := NewSettingService(repo, &config.Config{})
			compact, err := svc.GetCompactPublicSettings(context.Background())
			require.NoError(t, err)
			require.Less(t, len(compact.ContactInfo), 2048)
			require.Equal(t, 6, strings.Count(compact.ContactInfo, publicContactImagePath))
			require.NotContains(t, compact.ContactInfo, "data:image/")
			require.Contains(t, compact.ContactInfo, `"extra":9007199254740993`)
			require.Contains(t, compact.ContactInfo, fmt.Sprintf(`"enabled":%t`, enabled))
			legacy, err := svc.GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, raw, legacy.ContactInfo)
			require.Equal(t, raw, repo.values[SettingKeyContactInfo])

			injected, err := svc.GetPublicSettingsForInjection(context.Background())
			require.NoError(t, err)
			payload, ok := injected.(*PublicSettingsInjectionPayload)
			require.True(t, ok)
			require.Equal(t, compact.ContactInfo, payload.ContactInfo)

			contentType, data, ok, err := svc.GetPublicContactImageAsset(context.Background(), publicAssetRevision(image))
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, "image/png", contentType)
			require.Equal(t, imageBytes, data)

			repo.values[SettingKeyContactInfo] = "changed contact"
			svc.invalidatePublicSettingsCache()
			_, _, ok, err = svc.GetPublicContactImageAsset(context.Background(), publicAssetRevision(image))
			require.NoError(t, err)
			require.False(t, ok, "removed images must not be served under old revisions")
		})
	}
}

func TestCompactContactInfoPreservesOtherValues(t *testing.T) {
	for _, raw := range []string{
		"", "QQ: 12345", "{broken", "null", "[]",
		`{"version":"1","tabs":[]}`,
		`{"version":2,"tabs":[{"id":"service","cards":[{"image":"data:image/png;base64,AAAA"}]}]}`,
		`{"version":1,"tabs":[]} trailing`,
		`{"version":1,"tabs":[{"id":"service","cards":[{"image":"https://cdn.example.com/qr.png"},{"image":"/existing/qr.png"}]}]}`,
		`{"version":1,"tabs":[{"id":"service","cards":[{"image":"data:image/svg+xml,%3Csvg%3E%3C/svg%3E"}]}]}`,
	} {
		require.Equal(t, raw, compactContactInfo(raw))
	}
}

func TestSettingService_ContactImageRejectsInvalidContent(t *testing.T) {
	for _, image := range []string{
		"data:image/png;base64,not-base64",
		"data:image/png;base64,",
		"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<html>not an image</html>")),
		"data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0}, (2<<20)+1)),
		"data:image/svg+xml,%3Csvg%3E%3C/svg%3E",
		"https://cdn.example.com/qr.png",
	} {
		raw := fmt.Sprintf(`{"version":1,"tabs":[{"id":"service","cards":[{"image":%q}]}]}`, image)
		svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{SettingKeyContactInfo: raw}}, &config.Config{})
		_, _, ok, err := svc.GetPublicContactImageAsset(context.Background(), publicAssetRevision(image))
		require.NoError(t, err)
		require.False(t, ok)
	}

	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{})
	for _, revision := range []string{"", "stale", "zzzzzzzzzzzzzzzz", "0000000000000000"} {
		_, _, ok, err := svc.GetPublicContactImageAsset(context.Background(), revision)
		require.NoError(t, err)
		require.False(t, ok)
	}
}
