package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type asyncImageMemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]*service.ImageTaskRecord
}

func (s *asyncImageMemoryStore) Save(_ context.Context, task *service.ImageTaskRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	s.tasks[task.ID] = &copy
	return nil
}

func (s *asyncImageMemoryStore) Get(_ context.Context, id string) (*service.ImageTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task := s.tasks[id]
	if task == nil {
		return nil, service.ErrImageTaskNotFound
	}
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	return &copy, nil
}

func TestAsyncImageHandlerSubmitAndPoll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	release := make(chan struct{})
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		<-release
		c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"url": "https://example.test/image.png"}}})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`)).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "3", w.Header().Get("Retry-After"))

	var accepted struct {
		TaskID  string `json:"task_id"`
		Status  string `json:"status"`
		PollURL string `json:"poll_url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))
	require.Equal(t, service.ImageTaskStatusProcessing, accepted.Status)
	require.Equal(t, "/v1/images/tasks/"+accepted.TaskID, accepted.PollURL)
	require.Equal(t, accepted.PollURL, w.Header().Get("Location"))

	// The detached background request must survive completion of/cancellation
	// from the short submission request.
	cancelRequest()
	close(release)
	require.Eventually(t, func() bool {
		got, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
		return err == nil && got.Status == service.ImageTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)

	pollReq := httptest.NewRequest(http.MethodGet, accepted.PollURL, nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusOK, pollWriter.Code)
	require.Equal(t, "no-store", pollWriter.Header().Get("Cache-Control"))
	require.Empty(t, pollWriter.Header().Get("Retry-After"))
	require.Contains(t, pollWriter.Body.String(), "https://example.test/image.png")
}

func TestAsyncImageHandlerCompositeDispatchesResolvedPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type dispatchCapture struct {
		platform       string
		targetPlatform string
		upstreamModel  string
		publicModel    string
		path           string
		contentType    string
		body           []byte
		err            error
	}

	platforms := []string{service.PlatformOpenAI, service.PlatformGrok, service.PlatformGemini}
	formats := []string{"json", "multipart"}
	for _, platform := range platforms {
		for _, format := range formats {
			t.Run(platform+"/"+format, func(t *testing.T) {
				store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
				tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
				h := NewAsyncImageHandler(tasks, nil)
				// supportsPlatform requires the production Gemini forwarder wiring even
				// though this test replaces the executor with a dispatch spy.
				h.SetGeminiForwarder(&fakeGeminiImageForwarder{})

				captured := make(chan dispatchCapture, 1)
				h.execute = func(gotPlatform string, c *gin.Context) {
					body, err := io.ReadAll(c.Request.Body)
					targetPlatform, _ := service.ResolvedTargetPlatformFromContext(c.Request.Context())
					upstreamModel, _ := service.ResolvedUpstreamModelFromContext(c.Request.Context())
					publicModel, _ := service.RequestedPublicModelFromContext(c.Request.Context())
					captured <- dispatchCapture{
						platform:       gotPlatform,
						targetPlatform: targetPlatform,
						upstreamModel:  upstreamModel,
						publicModel:    publicModel,
						path:           c.Request.URL.Path,
						contentType:    c.GetHeader("Content-Type"),
						body:           body,
						err:            err,
					}
					c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"url": "https://example.test/image.png"}}})
				}

				const publicModel = "studio-image-alias"
				upstreamModel := platform + "-image-upstream"
				router := gin.New()
				router.Use(func(c *gin.Context) {
					groupID := int64(71)
					c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
						ID:      9,
						UserID:  7,
						GroupID: &groupID,
						Group: &service.Group{
							ID:                   groupID,
							Platform:             service.PlatformComposite,
							AllowImageGeneration: true,
						},
					})
					decision := service.CompositeRouteDecision{
						Matched:        true,
						Source:         service.CompositeRouteSourceExplicit,
						GroupID:        groupID,
						PublicModel:    publicModel,
						TargetPlatform: platform,
						UpstreamModel:  upstreamModel,
					}
					c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), decision))
					c.Next()
				})
				router.POST("/v1/images/generations/async", h.Submit)

				var requestBody []byte
				var contentType string
				if format == "json" {
					// The route middleware rewrites JSON before Submit runs.
					requestBody = []byte(`{"model":"` + upstreamModel + `","prompt":"cat"}`)
					contentType = "application/json"
				} else {
					var body bytes.Buffer
					writer := multipart.NewWriter(&body)
					require.NoError(t, writer.WriteField("model", publicModel))
					require.NoError(t, writer.WriteField("prompt", "cat"))
					require.NoError(t, writer.Close())
					requestBody = body.Bytes()
					contentType = writer.FormDataContentType()
				}

				req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", bytes.NewReader(requestBody))
				req.Header.Set("Content-Type", contentType)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, http.StatusAccepted, w.Code)

				var accepted struct {
					TaskID string `json:"task_id"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))
				require.NotEmpty(t, accepted.TaskID)

				var got dispatchCapture
				select {
				case got = <-captured:
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for async image dispatch")
				}
				require.NoError(t, got.err)
				require.Equal(t, platform, got.platform)
				require.Equal(t, platform, got.targetPlatform)
				require.Equal(t, upstreamModel, got.upstreamModel)
				require.Equal(t, publicModel, got.publicModel)
				require.Equal(t, "/v1/images/generations", got.path)

				if format == "json" {
					require.JSONEq(t, string(requestBody), string(got.body))
				} else {
					mediaType, params, err := mime.ParseMediaType(got.contentType)
					require.NoError(t, err)
					require.Equal(t, "multipart/form-data", mediaType)
					reader := multipart.NewReader(bytes.NewReader(got.body), params["boundary"])
					fields := make(map[string]string)
					for {
						part, err := reader.NextPart()
						if err == io.EOF {
							break
						}
						require.NoError(t, err)
						value, err := io.ReadAll(part)
						require.NoError(t, err)
						fields[part.FormName()] = string(value)
						require.NoError(t, part.Close())
					}
					require.Equal(t, publicModel, fields["model"])
					require.Equal(t, "cat", fields["prompt"])
				}

				require.Eventually(t, func() bool {
					task, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
					return err == nil && task.Status == service.ImageTaskStatusCompleted
				}, time.Second, 10*time.Millisecond)
			})
		}
	}
}

// fakeGeminiImageForwarder stands in for GatewayHandler.GeminiImages: it records
// the dispatched request and writes an OpenAI Images-shaped payload, which is
// what the real forwarder emits after converting the Gemini response.
type fakeGeminiImageForwarder struct {
	mu    sync.Mutex
	calls int
	path  string
}

func (f *fakeGeminiImageForwarder) GeminiImages(c *gin.Context) {
	f.mu.Lock()
	f.calls++
	f.path = c.Request.URL.Path
	f.mu.Unlock()
	c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"b64_json": "aGk=", "mime_type": "image/png"}}})
}

// A gemini-platform group must flow through the same async task pipeline as
// openai/grok once a forwarder is wired: 202 → detached execution → completed
// task whose result keeps the OpenAI Images shape (the S3 offload contract).
func TestAsyncImageHandlerGeminiSubmitDispatchesForwarder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	forwarder := &fakeGeminiImageForwarder{}
	h := NewAsyncImageHandler(tasks, nil)
	h.SetGeminiForwarder(forwarder)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(67)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformGemini, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gemini-3.1-flash-image","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	var accepted struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))

	require.Eventually(t, func() bool {
		got, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
		return err == nil && got.Status == service.ImageTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)

	forwarder.mu.Lock()
	calls, path := forwarder.calls, forwarder.path
	forwarder.mu.Unlock()
	require.Equal(t, 1, calls)
	// The executor must hand the forwarder the synchronous endpoint path.
	require.Equal(t, "/v1/images/generations", path)

	pollReq := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/"+accepted.TaskID, nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusOK, pollWriter.Code)
	require.Contains(t, pollWriter.Body.String(), `"b64_json"`)
}

// Without a wired forwarder gemini keeps the previous "not supported" behavior
// instead of accepting tasks that could never execute.
func TestAsyncImageHandlerGeminiWithoutForwarderReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	h := NewAsyncImageHandler(tasks, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(67)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformGemini, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gemini-3.1-flash-image","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "Images API is not supported for this platform")
	require.Empty(t, store.tasks)
}

// When object storage is not configured the feature is fully disabled: the
// endpoints must return 404 without creating a task or writing to Redis.
func TestAsyncImageHandlerDisabledReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithOptions(store, time.Hour, time.Minute) // enabled == false
	h := &AsyncImageHandler{tasks: tasks}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "not enabled")

	pollReq := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/imgtask_missing", nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusNotFound, pollWriter.Code)

	// No task was created / persisted.
	require.Empty(t, store.tasks)
}
