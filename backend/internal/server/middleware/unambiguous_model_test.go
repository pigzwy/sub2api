package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUnambiguousModelRejectsBeforeRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/responses", "/responses", "/backend-api/codex/responses", "/v1/responses/compact", "/v1/chat/completions", "/v1/messages", "/v1/embeddings", "/v1/live"} {
		for _, body := range []string{
			`{"model":"astra","model":"sol","input":"hello"}`,
			`{"model":"sol","model":"astra","messages":[{"role":"user","content":"hello"}]}`,
			`{"model":"sol","Model":"sol"}`,
			`{"model":"sol","\u006dodel":null}`,
			`{"session":{"model":"sol","Model":"astra"}}`,
		} {
			t.Run(path+body, func(t *testing.T) {
				router := gin.New()
				router.Use(UnambiguousModel(), GroupModelAllowlist()) // no group: allowlist disabled
				called := false
				router.POST(path, func(c *gin.Context) { called = true; c.Status(200) })
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, req)
				if w.Code != 400 || called {
					t.Fatalf("status=%d downstream=%v body=%s", w.Code, called, w.Body.String())
				}
			})
		}
	}
}

func TestUnambiguousModelPreservesBodyAndLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, body string
		compressed bool
		limit      int64
		status     int
	}{
		{"unchanged", `{ "model":"sol", "input":"hello", "unknown":1 }`, false, 1024, 200},
		{"gzip duplicate", `{"model":"sol","Model":"astra"}`, true, 1024, 400},
		{"gzip single", `{"model":"sol","input":"hello"}`, true, 1024, 200},
		{"body limit", `{"model":"sol","input":"hello"}`, false, 8, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(RequestBodyLimit(tc.limit), UnambiguousModel())
			r.POST("/v1/responses", func(c *gin.Context) {
				b, err := io.ReadAll(c.Request.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != tc.body {
					t.Fatalf("body changed: %s", b)
				}
				c.Status(200)
			})
			payload := []byte(tc.body)
			if tc.compressed {
				var buf bytes.Buffer
				w := gzip.NewWriter(&buf)
				_, _ = w.Write(payload)
				_ = w.Close()
				payload = buf.Bytes()
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			if tc.compressed {
				req.Header.Set("Content-Encoding", "gzip")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d", w.Code, tc.status)
			}
		})
	}
}

func TestUnambiguousModelRejectsRepeatedQuery(t *testing.T) {
	r := gin.New()
	r.Use(UnambiguousModel())
	r.GET("/v1/realtime", func(c *gin.Context) { c.Status(200) })
	for _, query := range []string{"model=sol&model=astra", "model=sol&Model=astra", "%6dodel=sol&model=astra"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/realtime?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("query=%s status=%d", query, w.Code)
		}
	}
}
