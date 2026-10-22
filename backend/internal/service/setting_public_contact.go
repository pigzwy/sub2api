package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
)

const publicContactImagePath = "/api/v1/settings/public/contact-image/"

// transformContactImages only touches image strings in the v1 contact form.
// Preserve legacy notes, unsupported versions and unknown fields, including
// their numeric precision. No raw settings or cached settings are mutated.
func transformContactImages(raw string, transform func(string) string) string {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return raw
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return raw
	}
	version, ok := document["version"].(json.Number)
	if !ok {
		return raw
	}
	v, err := version.Float64()
	if err != nil || v != 1 {
		return raw
	}
	tabs, _ := document["tabs"].([]any)
	changed := false
	for _, value := range tabs {
		tab, ok := value.(map[string]any)
		if !ok {
			continue
		}
		switch tab["id"] {
		case "service", "support", "business":
		default:
			continue
		}
		cards, _ := tab["cards"].([]any)
		for i, value := range cards {
			if i >= 2 {
				break
			}
			card, ok := value.(map[string]any)
			if !ok {
				continue
			}
			image, ok := card["image"].(string)
			if !ok {
				continue
			}
			if replacement := transform(image); replacement != image {
				card["image"] = replacement
				changed = true
			}
		}
	}
	if !changed {
		return raw
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func compactContactInfo(raw string) string {
	return transformContactImages(raw, func(image string) string {
		trimmed := strings.TrimSpace(image)
		if !supportedInlineLogoDataURL(trimmed) {
			return image
		}
		return publicContactImagePath + publicAssetRevision(trimmed)
	})
}

// GetPublicContactImageAsset only serves an image still present in the current
// contact configuration. Content-addressed URLs remain stable across tab edits.
func (s *SettingService) GetPublicContactImageAsset(ctx context.Context, revision string) (string, []byte, bool, error) {
	if len(revision) != publicAssetRevisionLength {
		return "", nil, false, nil
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return "", nil, false, nil
	}
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return "", nil, false, err
	}
	var matched string
	_ = transformContactImages(settings.ContactInfo, func(image string) string {
		trimmed := strings.TrimSpace(image)
		if supportedInlineLogoDataURL(trimmed) && publicAssetRevision(trimmed) == revision {
			matched = trimmed
		}
		return image
	})
	contentType, data, ok := decodePublicImageDataURL(matched)
	return contentType, data, ok, nil
}
