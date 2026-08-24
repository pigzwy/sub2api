package service

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The GPT image family rejects response_format outright ("Unknown parameter"),
// while dall-e needs it to return base64 instead of a URL. These tests pin that
// split on all three paths that can put the field on an upstream request:
// the JSON forward, the multipart (/images/edits) forward, and the admin test.

func TestRewriteOpenAIImagesModel_DropsResponseFormatForGPTImage(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2","prompt":"a red circle","n":1,"response_format":"b64_json"}`)

	out, contentType, err := rewriteOpenAIImagesModel(body, "application/json", "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.NotContains(t, string(out), "response_format")
	// The rest of the request must survive untouched.
	require.Contains(t, string(out), `"model":"gpt-image-2"`)
	require.Contains(t, string(out), `"prompt":"a red circle"`)
}

func TestRewriteOpenAIImagesModel_KeepsResponseFormatForDallE(t *testing.T) {
	body := []byte(`{"model":"dall-e-3","prompt":"a red circle","response_format":"b64_json"}`)

	out, _, err := rewriteOpenAIImagesModel(body, "application/json", "dall-e-3")
	require.NoError(t, err)
	require.Contains(t, string(out), `"response_format":"b64_json"`)
}

func TestRewriteOpenAIImagesModel_DropsMultipartResponseFormatForGPTImage(t *testing.T) {
	for _, tc := range []struct {
		name        string
		model       string
		wantDropped bool
	}{
		{name: "gpt image drops the field", model: "gpt-image-2", wantDropped: true},
		{name: "dall-e keeps the field", model: "dall-e-2", wantDropped: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)
			require.NoError(t, writer.WriteField("model", "placeholder"))
			require.NoError(t, writer.WriteField("prompt", "a red circle"))
			require.NoError(t, writer.WriteField("response_format", "b64_json"))
			filePart, err := writer.CreateFormFile("image", "source.png")
			require.NoError(t, err)
			_, err = filePart.Write([]byte("fake-png-bytes"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())

			out, contentType, err := rewriteOpenAIImagesModel(buf.Bytes(), writer.FormDataContentType(), tc.model)
			require.NoError(t, err)

			fields, files := readMultipartForTest(t, out, contentType)
			if tc.wantDropped {
				require.NotContains(t, fields, "response_format")
			} else {
				require.Equal(t, "b64_json", fields["response_format"])
			}
			// The model is rewritten and every other part is relayed verbatim.
			require.Equal(t, tc.model, fields["model"])
			require.Equal(t, "a red circle", fields["prompt"])
			require.Equal(t, "fake-png-bytes", files["image"])
		})
	}
}

// readMultipartForTest returns the plain fields and the file parts of a multipart body.
func readMultipartForTest(t *testing.T, body []byte, contentType string) (map[string]string, map[string]string) {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])

	fields := map[string]string{}
	files := map[string]string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FileName() != "" {
			files[part.FormName()] = string(data)
		} else {
			fields[part.FormName()] = string(data)
		}
		require.NoError(t, part.Close())
	}
	return fields, files
}

func TestAccountTestService_OpenAIImageAPIKeyOmitsResponseFormatForGPTImage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		model     string
		wantField bool
	}{
		{name: "gpt image omits the field", model: "gpt-image-2", wantField: false},
		{name: "dall-e still asks for base64", model: "dall-e-3", wantField: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

			upstream := &httpUpstreamRecorder{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aGVsbG8="}]}`)),
				},
			}
			svc := &AccountTestService{httpUpstream: upstream, cfg: &config.Config{}}
			account := &Account{
				ID:       54,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					"api_key":  "test-api-key",
					"base_url": "https://image-upstream.example/v1",
				},
			}

			require.NoError(t, svc.testOpenAIImageAPIKey(c, context.Background(), account, tc.model, "draw a cat"))
			require.NotNil(t, upstream.lastReq)

			sent := string(upstream.lastBody)
			if tc.wantField {
				require.Contains(t, sent, `"response_format":"b64_json"`)
			} else {
				require.NotContains(t, sent, "response_format")
			}
			require.Contains(t, sent, `"prompt":"draw a cat"`)
		})
	}
}
