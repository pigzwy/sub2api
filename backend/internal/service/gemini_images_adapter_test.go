//go:build unit

package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildGeminiImageGenerateRequest_PromptOnly(t *testing.T) {
	body, err := BuildGeminiImageGenerateRequest(&OpenAIImagesRequest{
		Model:    "gemini-3.1-flash-image",
		Prompt:   "a cat riding a bike",
		SizeTier: "1K",
	})
	require.NoError(t, err)

	require.Equal(t, "user", gjson.GetBytes(body, "contents.0.role").String())
	require.Equal(t, "a cat riding a bike", gjson.GetBytes(body, "contents.0.parts.0.text").String())
	require.Equal(t, "1K", gjson.GetBytes(body, "generationConfig.imageConfig.imageSize").String())

	modalities := gjson.GetBytes(body, "generationConfig.responseModalities").Array()
	require.Len(t, modalities, 2)
	require.Equal(t, "TEXT", modalities[0].String())
	require.Equal(t, "IMAGE", modalities[1].String())
}

func TestBuildGeminiImageGenerateRequest_SizeFromPixelDimensions(t *testing.T) {
	body, err := BuildGeminiImageGenerateRequest(&OpenAIImagesRequest{
		Prompt: "hi",
		Size:   "2048x2048", // classifies to 2K
	})
	require.NoError(t, err)
	require.Equal(t, "2K", gjson.GetBytes(body, "generationConfig.imageConfig.imageSize").String())
}

func TestBuildGeminiImageGenerateRequest_OmitsImageConfigWhenSizeUnknown(t *testing.T) {
	body, err := BuildGeminiImageGenerateRequest(&OpenAIImagesRequest{
		Prompt: "hi",
		Size:   "auto", // does not classify → no imageConfig, billing defaults to 2K
	})
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "generationConfig.imageConfig").Exists())
}

func TestBuildGeminiImageGenerateRequest_AspectRatioPassthrough(t *testing.T) {
	body, err := BuildGeminiImageGenerateRequest(&OpenAIImagesRequest{
		Prompt: "hi",
		Body:   []byte(`{"prompt":"hi","aspect_ratio":"16:9"}`),
	})
	require.NoError(t, err)
	require.Equal(t, "16:9", gjson.GetBytes(body, "generationConfig.imageConfig.aspectRatio").String())
}

func TestBuildGeminiImageGenerateRequest_EditWithInlineImage(t *testing.T) {
	raw := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a} // PNG magic
	body, err := BuildGeminiImageGenerateRequest(&OpenAIImagesRequest{
		Prompt: "make it blue",
		Uploads: []OpenAIImagesUpload{
			{FieldName: "image", ContentType: "image/png", Data: raw},
		},
	})
	require.NoError(t, err)

	// parts[0] is the prompt text, parts[1] the inline image.
	require.Equal(t, "make it blue", gjson.GetBytes(body, "contents.0.parts.0.text").String())
	require.Equal(t, "image/png", gjson.GetBytes(body, "contents.0.parts.1.inlineData.mimeType").String())
	require.Equal(t, base64.StdEncoding.EncodeToString(raw), gjson.GetBytes(body, "contents.0.parts.1.inlineData.data").String())
}

func TestBuildGeminiImageGenerateRequest_DataURLInputImage(t *testing.T) {
	raw := []byte{0xff, 0xd8, 0xff, 0xe0} // JPEG magic
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
	body, err := BuildGeminiImageGenerateRequest(&OpenAIImagesRequest{
		Prompt:         "edit",
		InputImageURLs: []string{dataURL},
	})
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", gjson.GetBytes(body, "contents.0.parts.1.inlineData.mimeType").String())
	require.Equal(t, base64.StdEncoding.EncodeToString(raw), gjson.GetBytes(body, "contents.0.parts.1.inlineData.data").String())
}

func TestBuildGeminiImageGenerateRequest_EmptyIsError(t *testing.T) {
	_, err := BuildGeminiImageGenerateRequest(&OpenAIImagesRequest{})
	require.ErrorIs(t, err, ErrGeminiImageEmptyRequest)

	_, err = BuildGeminiImageGenerateRequest(nil)
	require.ErrorIs(t, err, ErrGeminiImageEmptyRequest)
}

func TestConvertGeminiImageResponseToOpenAI_InlineImage(t *testing.T) {
	img := base64.StdEncoding.EncodeToString([]byte("fake-image-bytes"))
	gemini := []byte(`{
		"candidates":[{"content":{"parts":[
			{"text":"here you go"},
			{"inlineData":{"mimeType":"image/png","data":"` + img + `"}}
		]}}]
	}`)

	out, count, err := ConvertGeminiImageResponseToOpenAI(gemini, 1700000000)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, int64(1700000000), gjson.GetBytes(out, "created").Int())
	require.Equal(t, img, gjson.GetBytes(out, "data.0.b64_json").String())
	require.Equal(t, "image/png", gjson.GetBytes(out, "data.0.mime_type").String())
	// The b64_json field is what ImageResultUploader.Rewrite consumes for S3 offload.
	require.True(t, gjson.GetBytes(out, "data.0.b64_json").Exists())
}

func TestConvertGeminiImageResponseToOpenAI_SnakeCaseInlineData(t *testing.T) {
	img := base64.StdEncoding.EncodeToString([]byte("bytes"))
	gemini := []byte(`{"candidates":[{"content":{"parts":[{"inline_data":{"mime_type":"image/webp","data":"` + img + `"}}]}}]}`)
	out, count, err := ConvertGeminiImageResponseToOpenAI(gemini, 42)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, img, gjson.GetBytes(out, "data.0.b64_json").String())
	require.Equal(t, "image/webp", gjson.GetBytes(out, "data.0.mime_type").String())
}

func TestConvertGeminiImageResponseToOpenAI_MultipleImages(t *testing.T) {
	a := base64.StdEncoding.EncodeToString([]byte("a"))
	b := base64.StdEncoding.EncodeToString([]byte("b"))
	gemini := []byte(`{"candidates":[
		{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + a + `"}}]}},
		{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + b + `"}}]}}
	]}`)
	out, count, err := ConvertGeminiImageResponseToOpenAI(gemini, 0)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Len(t, gjson.GetBytes(out, "data").Array(), 2)
}

func TestConvertGeminiImageResponseToOpenAI_BlockReason(t *testing.T) {
	gemini := []byte(`{"promptFeedback":{"blockReason":"SAFETY"}}`)
	_, _, err := ConvertGeminiImageResponseToOpenAI(gemini, 0)
	require.ErrorIs(t, err, ErrGeminiImageBlocked)
	require.Contains(t, err.Error(), "SAFETY")
}

func TestConvertGeminiImageResponseToOpenAI_SafetyFinishReasonNoImage(t *testing.T) {
	gemini := []byte(`{"candidates":[{"finishReason":"IMAGE_SAFETY","content":{"parts":[]}}]}`)
	_, _, err := ConvertGeminiImageResponseToOpenAI(gemini, 0)
	require.ErrorIs(t, err, ErrGeminiImageBlocked)
	require.Contains(t, err.Error(), "IMAGE_SAFETY")
}

func TestConvertGeminiImageResponseToOpenAI_NoImageTextOnly(t *testing.T) {
	gemini := []byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"I can't do that"}]}}]}`)
	_, _, err := ConvertGeminiImageResponseToOpenAI(gemini, 0)
	require.ErrorIs(t, err, ErrGeminiImageNoOutput)
}

func TestConvertGeminiImageResponseToOpenAI_InvalidJSON(t *testing.T) {
	_, _, err := ConvertGeminiImageResponseToOpenAI([]byte("not json"), 0)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrGeminiImageNoOutput))
}

func TestConvertGeminiImageResponseToOpenAI_IgnoresEmptyInlineData(t *testing.T) {
	// An inlineData part with no data must not be counted as an image.
	gemini := []byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":""}}]}}]}`)
	_, _, err := ConvertGeminiImageResponseToOpenAI(gemini, 0)
	require.ErrorIs(t, err, ErrGeminiImageNoOutput)
}

// Guard: the produced OpenAI response must be shaped so ImageResultUploader.Rewrite
// can walk data[].b64_json (the S3 offload contract).
func TestConvertGeminiImageResponseToOpenAI_ShapeMatchesUploaderContract(t *testing.T) {
	img := base64.StdEncoding.EncodeToString([]byte("x"))
	gemini := []byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + img + `"}}]}}]}`)
	out, _, err := ConvertGeminiImageResponseToOpenAI(gemini, 0)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &top))
	require.Contains(t, top, "data")
	var items []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["data"], &items))
	require.Len(t, items, 1)
	require.Contains(t, items[0], "b64_json")
}
