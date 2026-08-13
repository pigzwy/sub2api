package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// gemini_images_adapter.go bridges the OpenAI Images API surface to Gemini's
// native generateContent request/response shapes. It lets gemini-platform image
// models (Nano Banana: gemini-2.5-flash-image / gemini-3.1-flash-image /
// gemini-3-pro-image) flow through the same /v1/images pipeline (and its async
// S3 offload) as gpt-image and grok, instead of only working on the raw
// /v1beta generateContent path.
//
// The functions here are intentionally pure (no gin/context) so the request and
// response mapping can be unit tested in isolation.

// Sentinel errors let the handler pick an HTTP status without string matching.
var (
	// ErrGeminiImageEmptyRequest is returned when the OpenAI request carries
	// neither a prompt nor an input image, so there is nothing to send upstream.
	ErrGeminiImageEmptyRequest = errors.New("image request has neither prompt nor input image")
	// ErrGeminiImageBlocked is returned when the upstream declined to generate
	// due to a safety/prompt block. Callers should surface it as a client error.
	ErrGeminiImageBlocked = errors.New("image generation was blocked by the upstream")
	// ErrGeminiImageNoOutput is returned when a 2xx upstream response contained
	// no inline image part (e.g. a text-only refusal or a fileData reference).
	ErrGeminiImageNoOutput = errors.New("upstream returned no inline image")
)

// geminiImageResponseModalities is the modality set required to make a Gemini
// image model emit an inline image alongside any text.
var geminiImageResponseModalities = []string{"TEXT", "IMAGE"}

// BuildGeminiImageGenerateRequest maps a parsed OpenAI Images request to a
// Gemini generateContent request body.
//
//   - prompt              → contents[0].parts[{text}]
//   - edit input image(s) → contents[0].parts[{inlineData:{mimeType,data}}]
//   - size tier           → generationConfig.imageConfig.imageSize (also the
//     billing tier source, read back by GeminiMessagesCompatService)
//   - aspect_ratio        → generationConfig.imageConfig.aspectRatio (passthrough)
//   - responseModalities  → ["TEXT","IMAGE"] so an image part is returned
//
// n is deliberately NOT mapped to candidateCount: several Gemini image models
// reject candidateCount>1 with a 400, so we ask for the default single image and
// bill by the inline images actually returned. Callers that need n>1 should loop.
func BuildGeminiImageGenerateRequest(parsed *OpenAIImagesRequest) ([]byte, error) {
	if parsed == nil {
		return nil, ErrGeminiImageEmptyRequest
	}

	parts := make([]map[string]any, 0, 1+len(parsed.Uploads))
	if prompt := strings.TrimSpace(parsed.Prompt); prompt != "" {
		parts = append(parts, map[string]any{"text": prompt})
	}
	for _, upload := range parsed.Uploads {
		if len(upload.Data) == 0 {
			continue
		}
		mimeType := strings.TrimSpace(upload.ContentType)
		if mimeType == "" {
			mimeType = detectImageContentType(upload.Data)
		}
		parts = append(parts, map[string]any{
			"inlineData": map[string]any{
				"mimeType": mimeType,
				"data":     base64.StdEncoding.EncodeToString(upload.Data),
			},
		})
	}
	// Also accept data: URLs supplied via the JSON body (non-multipart edits).
	for _, rawURL := range parsed.InputImageURLs {
		mimeType, data, ok := decodeDataImageURL(rawURL)
		if !ok {
			continue
		}
		parts = append(parts, map[string]any{
			"inlineData": map[string]any{
				"mimeType": mimeType,
				"data":     base64.StdEncoding.EncodeToString(data),
			},
		})
	}

	if len(parts) == 0 {
		return nil, ErrGeminiImageEmptyRequest
	}

	imageConfig := map[string]any{}
	if tier := geminiImageSizeTier(parsed); tier != "" {
		imageConfig["imageSize"] = tier
	}
	if ratio := geminiImageAspectRatio(parsed); ratio != "" {
		imageConfig["aspectRatio"] = ratio
	}

	generationConfig := map[string]any{
		"responseModalities": geminiImageResponseModalities,
	}
	if len(imageConfig) > 0 {
		generationConfig["imageConfig"] = imageConfig
	}

	payload := map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": parts},
		},
		"generationConfig": generationConfig,
	}
	return json.Marshal(payload)
}

// geminiImageSizeTier resolves the billing/size tier ("1K"/"2K"/"4K") from the
// parsed request, preferring the already-classified tier and falling back to the
// raw size string. Empty means "let the upstream/billing default apply" (2K).
func geminiImageSizeTier(parsed *OpenAIImagesRequest) string {
	if tier := strings.TrimSpace(parsed.SizeTier); tier != "" {
		if normalized, ok := ClassifyImageBillingTier(tier); ok {
			return normalized
		}
	}
	if size := strings.TrimSpace(parsed.Size); size != "" {
		if normalized, ok := ClassifyImageBillingTier(size); ok {
			return normalized
		}
	}
	return ""
}

// geminiImageAspectRatio extracts an optional aspect_ratio hint from the raw
// request body (OpenAI Images has no standard field for it, but clients that
// target Gemini may pass one through).
func geminiImageAspectRatio(parsed *OpenAIImagesRequest) string {
	if parsed == nil || len(parsed.Body) == 0 || !gjson.ValidBytes(parsed.Body) {
		return ""
	}
	for _, key := range []string{"aspect_ratio", "aspectRatio"} {
		if v := strings.TrimSpace(gjson.GetBytes(parsed.Body, key).String()); v != "" {
			return v
		}
	}
	return ""
}

// decodeDataImageURL decodes a `data:image/...;base64,....` URL into its mime
// type and raw bytes. Non-data URLs return ok=false (http(s) references cannot
// be inlined without a fetch, which this adapter deliberately avoids).
func decodeDataImageURL(rawURL string) (string, []byte, bool) {
	rawURL = strings.TrimSpace(rawURL)
	const prefix = "data:"
	if len(rawURL) < len(prefix) || !strings.EqualFold(rawURL[:len(prefix)], prefix) {
		return "", nil, false
	}
	comma := strings.IndexByte(rawURL, ',')
	if comma < 0 {
		return "", nil, false
	}
	meta := rawURL[len(prefix):comma]
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return "", nil, false
	}
	mimeType := meta
	if idx := strings.IndexByte(meta, ';'); idx >= 0 {
		mimeType = meta[:idx]
	}
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rawURL[comma+1:]))
	if err != nil || len(data) == 0 {
		return "", nil, false
	}
	return mimeType, data, true
}

// ConvertGeminiImageResponseToOpenAI maps a Gemini generateContent response into
// the OpenAI Images response shape ({created, data:[{b64_json, mime_type}]}).
//
// The b64_json field is what the shared async pipeline's object-storage offload
// (ImageResultUploader.Rewrite) consumes, so a Gemini result reaches S3 exactly
// like gpt-image/grok. Returns the number of inline images mapped.
//
// A prompt/safety block maps to ErrGeminiImageBlocked; a 2xx-but-imageless
// response maps to ErrGeminiImageNoOutput.
func ConvertGeminiImageResponseToOpenAI(geminiBody []byte, created int64) ([]byte, int, error) {
	if len(geminiBody) == 0 || !gjson.ValidBytes(geminiBody) {
		return nil, 0, fmt.Errorf("parse gemini image response: invalid JSON")
	}

	if reason := strings.TrimSpace(gjson.GetBytes(geminiBody, "promptFeedback.blockReason").String()); reason != "" {
		return nil, 0, fmt.Errorf("%w: %s", ErrGeminiImageBlocked, reason)
	}

	type imageItem struct {
		B64JSON  string `json:"b64_json"`
		MimeType string `json:"mime_type,omitempty"`
	}
	var items []imageItem

	gjson.GetBytes(geminiBody, "candidates").ForEach(func(_, candidate gjson.Result) bool {
		candidate.Get("content.parts").ForEach(func(_, part gjson.Result) bool {
			inline := part.Get("inlineData")
			if !inline.Exists() {
				inline = part.Get("inline_data")
			}
			if !inline.Exists() {
				return true
			}
			data := strings.TrimSpace(inline.Get("data").String())
			if data == "" {
				return true
			}
			mimeType := inline.Get("mimeType")
			if !mimeType.Exists() {
				mimeType = inline.Get("mime_type")
			}
			items = append(items, imageItem{
				B64JSON:  data,
				MimeType: strings.TrimSpace(mimeType.String()),
			})
			return true
		})
		return true
	})

	if len(items) == 0 {
		// Distinguish an explicit safety finish from a generic empty result so the
		// caller can still surface something actionable.
		if reason := strings.TrimSpace(gjson.GetBytes(geminiBody, "candidates.0.finishReason").String()); reason != "" &&
			!strings.EqualFold(reason, "STOP") {
			return nil, 0, fmt.Errorf("%w: %s", ErrGeminiImageBlocked, reason)
		}
		return nil, 0, ErrGeminiImageNoOutput
	}

	out := map[string]any{
		"created": created,
		"data":    items,
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, 0, fmt.Errorf("encode openai image response: %w", err)
	}
	return encoded, len(items), nil
}
