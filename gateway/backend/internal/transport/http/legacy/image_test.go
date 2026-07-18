package legacy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type fakeImageGenerator struct {
	calls      int
	inputs     []gateway.ImageGenerationInput
	editInputs []gateway.ImageEditInput
}

type fakeLegacyImageCache struct {
	items     []LegacyCachedImage
	listError error
}

type legacyImageStatusError struct{ status int }

func (e legacyImageStatusError) Error() string       { return "upstream image rejected" }
func (e legacyImageStatusError) HTTPStatusCode() int { return e.status }

func (f *fakeLegacyImageCache) ListImages() ([]LegacyCachedImage, error) {
	return f.items, f.listError
}

func TestWriteImagineEditErrorPreservesProviderStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	contextValue, _ := gin.CreateTestContext(recorder)
	writeImagineEditError(contextValue, false, legacyImageStatusError{status: http.StatusForbidden})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestImagineCacheListUsesPublicAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	imageCache := &fakeLegacyImageCache{items: []LegacyCachedImage{{
		Name: "cached-image.jpg", ViewURL: "/v1/files/image/cached-image.jpg",
		SizeBytes: 1234, ModifiedAtMS: 5678,
	}}}
	handler := NewHandler(Options{PublicEnabled: true, ImageCache: imageCache}, authenticator)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/public/imagine/cache/list?page=1&page_size=100", nil)
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{`"total":1`, `"name":"cached-image.jpg"`, `"view_url":"/v1/files/image/cached-image.jpg"`, `"size_bytes":1234`, `"mtime_ms":5678`} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("body missing %s: %s", expected, recorder.Body.String())
		}
	}
}

func TestImagineWebSocketStreamsExistingTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	generator := &fakeImageGenerator{}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, generator)
	router := gin.New()
	handler.Register(router, nil, nil)
	server := httptest.NewServer(router)
	defer server.Close()

	startRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/public/imagine/start", bytes.NewBufferString(`{"prompt":"draw ws","aspect_ratio":"1:1"}`))
	startRequest.Header.Set("Authorization", "Bearer g2-direct-key")
	startRequest.Header.Set("Content-Type", "application/json")
	startResponse, err := http.DefaultClient.Do(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	startBody, _ := io.ReadAll(startResponse.Body)
	_ = startResponse.Body.Close()
	taskID := jsonStringField(t, startBody, "task_id")

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/public/imagine/ws?task_id=" + taskID + "&public_key=g2-direct-key"
	connection, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seenStatus, seenImage := false, false
	for index := 0; index < 4; index++ {
		_, message, readErr := connection.ReadMessage()
		if readErr != nil {
			break
		}
		seenStatus = seenStatus || strings.Contains(string(message), `"status":"running"`)
		seenImage = seenImage || strings.Contains(string(message), `"b64_json":"YWJj"`)
		if seenStatus && seenImage {
			break
		}
	}
	if !seenStatus || !seenImage {
		t.Fatalf("seen status=%v image=%v", seenStatus, seenImage)
	}
}

func (f *fakeImageGenerator) GenerateImage(_ context.Context, input gateway.ImageGenerationInput) (*gateway.Result, error) {
	f.calls++
	f.inputs = append(f.inputs, input)
	if f.calls > 1 {
		return nil, errors.New("stop test stream")
	}
	body := `{"created":123,"data":[{"b64_json":"YWJj","mime_type":"image/png"}]}`
	return &gateway.Result{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Finalize:   func(gateway.Usage, string, string) {},
	}, nil
}

func (f *fakeImageGenerator) EditImage(_ context.Context, input gateway.ImageEditInput) (*gateway.Result, error) {
	f.editInputs = append(f.editInputs, input)
	body := `{"created":456,"data":[{"b64_json":"ZWRpdA==","mime_type":"image/png"}]}`
	return &gateway.Result{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Finalize:   func(gateway.Usage, string, string) {},
	}, nil
}

func TestImagineConfigDoesNotRequirePageKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(Options{PublicEnabled: true, AllowNSFW: true}, nil)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/public/imagine/config", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"nsfw":true`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestImagineStartSSEAndStopUseGoImageGenerator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	generator := &fakeImageGenerator{}
	handler := NewHandler(Options{PublicEnabled: true, AllowNSFW: true}, authenticator, generator)
	router := gin.New()
	handler.Register(router, nil, nil)

	startRequest := httptest.NewRequest(http.MethodPost, "/v1/public/imagine/start", bytes.NewBufferString(`{"prompt":"draw","aspect_ratio":"16:9","nsfw":true,"pro":true}`))
	startRequest.Header.Set("Authorization", "Bearer g2-direct-key")
	startRequest.Header.Set("Content-Type", "application/json")
	startRecorder := httptest.NewRecorder()
	router.ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	taskID := jsonStringField(t, startRecorder.Body.Bytes(), "task_id")
	if taskID == "" {
		t.Fatal("missing task_id")
	}

	sseRequest := httptest.NewRequest(http.MethodGet, "/v1/public/imagine/sse?task_id="+taskID+"&public_key=g2-direct-key", nil)
	sseRecorder := httptest.NewRecorder()
	router.ServeHTTP(sseRecorder, sseRequest)
	if sseRecorder.Code != http.StatusOK {
		t.Fatalf("sse status=%d body=%s", sseRecorder.Code, sseRecorder.Body.String())
	}
	body := sseRecorder.Body.String()
	for _, expected := range []string{`"type":"status"`, `"status":"running"`, `"type":"image"`, `"b64_json":"YWJj"`, `"image_id":"local-`, `"current_source_image_url":"data:image/png;base64,YWJj"`, `"type":"error"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("SSE body missing %s: %s", expected, body)
		}
	}
	if len(generator.inputs) == 0 {
		t.Fatal("image generator was not called")
	}
	input := generator.inputs[0]
	if input.PublicModel != "grok-imagine-image-quality" || input.Prompt != "draw" || input.AspectRatio != "16:9" || input.Resolution != "2k" || input.ResponseFormat != "b64_json" || input.Count != 1 || input.NSFW == nil || !*input.NSFW {
		t.Fatalf("input = %#v", input)
	}

	secondStart := httptest.NewRequest(http.MethodPost, "/v1/public/imagine/start", bytes.NewBufferString(`{"prompt":"draw again"}`))
	secondStart.Header.Set("Authorization", "Bearer g2-direct-key")
	secondStart.Header.Set("Content-Type", "application/json")
	secondRecorder := httptest.NewRecorder()
	router.ServeHTTP(secondRecorder, secondStart)
	secondTaskID := jsonStringField(t, secondRecorder.Body.Bytes(), "task_id")

	stopRequest := httptest.NewRequest(http.MethodPost, "/v1/public/imagine/stop", bytes.NewBufferString(`{"task_ids":["`+secondTaskID+`"]}`))
	stopRequest.Header.Set("Authorization", "Bearer g2-direct-key")
	stopRequest.Header.Set("Content-Type", "application/json")
	stopRecorder := httptest.NewRecorder()
	router.ServeHTTP(stopRecorder, stopRequest)
	if stopRecorder.Code != http.StatusOK || !strings.Contains(stopRecorder.Body.String(), `"removed":1`) {
		t.Fatalf("stop status=%d body=%s", stopRecorder.Code, stopRecorder.Body.String())
	}
}

func TestImagineEditAndWorkbenchMapToGoMultiImageEditor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	generator := &fakeImageGenerator{}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, generator)
	router := gin.New()
	handler.Register(router, nil, nil)

	tests := []struct {
		name         string
		path         string
		body         string
		wantRefs     int
		wantStream   bool
		wantContains []string
	}{
		{
			name: "single edit", path: "/v1/public/imagine/edit",
			body:     `{"prompt":"change","parent_post_id":"parent-1","source_image_url":"data:image/png;base64,YWJj"}`,
			wantRefs: 1, wantContains: []string{`"b64_json":"ZWRpdA=="`, `"parent_post_id":"parent-1"`, `"current_parent_post_id":"parent-1"`},
		},
		{
			name: "stream edit", path: "/v1/public/imagine/edit",
			body:     `{"prompt":"change","parent_post_id":"parent-2","source_image_url":"data:image/png;base64,YWJj","stream":true}`,
			wantRefs: 1, wantStream: true, wantContains: []string{"event: progress", "event: result", `"b64_json":"ZWRpdA=="`},
		},
		{
			name: "parent only edit", path: "/v1/public/imagine/edit",
			body:     `{"prompt":"continue","parent_post_id":"123e4567-e89b-12d3-a456-426614174000"}`,
			wantRefs: 1, wantContains: []string{`"parent_post_id":"123e4567-e89b-12d3-a456-426614174000"`},
		},
		{
			name: "workbench parent ref", path: "/v1/public/imagine/workbench/edit",
			body:     `{"prompt":"continue","reference_items":[{"parent_post_id":"123e4567-e89b-12d3-a456-426614174000"}]}`,
			wantRefs: 1, wantContains: []string{`"b64_json":"ZWRpdA=="`},
		},
		{
			name: "workbench refs", path: "/v1/public/imagine/workbench/edit",
			body:     `{"prompt":"merge","image_references":["data:image/png;base64,YQ==","https://example.com/b.png"],"stream":true}`,
			wantRefs: 2, wantStream: true, wantContains: []string{"event: progress", "event: result", `"b64_json":"ZWRpdA=="`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Authorization", "Bearer g2-direct-key")
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if test.wantStream && !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/event-stream") {
				t.Fatalf("content-type=%q", recorder.Header().Get("Content-Type"))
			}
			for _, expected := range test.wantContains {
				if !strings.Contains(recorder.Body.String(), expected) {
					t.Fatalf("body missing %q: %s", expected, recorder.Body.String())
				}
			}
			last := generator.editInputs[len(generator.editInputs)-1]
			if last.PublicModel != "grok-imagine-image-edit" || last.Prompt == "" || last.Count != 1 || last.ResponseFormat != "b64_json" || len(last.ImageURLs) != test.wantRefs {
				t.Fatalf("input=%#v", last)
			}
		})
	}
}

func TestImagineParentPostReturnsDeterministicFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, &fakeImageGenerator{})
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/public/imagine/parent-post?parent_post_id=123e4567-e89b-12d3-a456-426614174000", nil)
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"source_image_url":"https://imagine-public.x.ai/imagine-public/images/123e4567-e89b-12d3-a456-426614174000.jpg"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodGet, "/v1/public/imagine/parent-post?parent_post_id=../../secret", nil)
	invalid.Header.Set("Authorization", "Bearer g2-direct-key")
	invalidRecorder := httptest.NewRecorder()
	router.ServeHTTP(invalidRecorder, invalid)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d body=%s", invalidRecorder.Code, invalidRecorder.Body.String())
	}
}

func jsonStringField(t *testing.T, body []byte, key string) string {
	t.Helper()
	marker := `"` + key + `":"`
	start := bytes.Index(body, []byte(marker))
	if start < 0 {
		return ""
	}
	value := body[start+len(marker):]
	end := bytes.IndexByte(value, '"')
	if end < 0 {
		return ""
	}
	return string(value[:end])
}
