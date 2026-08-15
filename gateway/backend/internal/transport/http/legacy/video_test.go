package legacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/gin-gonic/gin"
)

type fakeLegacyVideoGateway struct {
	created      []gateway.VideoInput
	createError  error
	polls        map[string]int
	cancelled    []string
	listed       []mediadomain.Job
	renamedID    string
	renamedTo    string
	renameError  error
	deletedID    string
	deleteResult bool
	deleteError  error
}

type fakeVideoReferenceStore struct {
	saved [][]byte
}

func (f *fakeVideoReferenceStore) SaveImage(_ context.Context, data []byte) (mediadomain.Asset, error) {
	f.saved = append(f.saved, append([]byte(nil), data...))
	return mediadomain.Asset{ID: "stored-reference-1"}, nil
}

func (f *fakeVideoReferenceStore) PublicImageURL(id string) string {
	return "https://grok.uonoe.com/v1/media/images/" + id
}

type fakeLegacyVideoCache struct {
	items     []LegacyCachedVideo
	renamedID string
	renamedTo string
	listError error
}

func (f *fakeLegacyVideoCache) ListVideos() ([]LegacyCachedVideo, error) {
	if f.listError != nil {
		return nil, f.listError
	}
	return f.items, nil
}

func (f *fakeLegacyVideoCache) RenameVideo(identifier, displayName string) (LegacyCachedVideo, error) {
	f.renamedID, f.renamedTo = identifier, displayName
	return LegacyCachedVideo{PostID: identifier, DisplayName: displayName, ViewURL: "/v1/files/video/local.mp4"}, nil
}

func (f *fakeLegacyVideoGateway) ListVideos(_ context.Context, _ clientkeydomain.Key, _, _ int) ([]mediadomain.Job, int64, error) {
	return f.listed, int64(len(f.listed)), nil
}

func (f *fakeLegacyVideoGateway) RenameVideo(_ context.Context, identifier, displayName string, _ clientkeydomain.Key) (mediadomain.Job, error) {
	if f.renameError != nil {
		return mediadomain.Job{}, f.renameError
	}
	f.renamedID, f.renamedTo = identifier, displayName
	return mediadomain.Job{ID: "video-job-1", UpstreamURL: "https://example.com/123e4567-e89b-12d3-a456-426614174000.mp4", InputJSON: `{"display_name":"` + displayName + `"}`}, nil
}

func (f *fakeLegacyVideoGateway) DeleteVideo(_ context.Context, id string, _ clientkeydomain.Key) (bool, error) {
	f.deletedID = id
	return f.deleteResult, f.deleteError
}

func TestVideoCacheDeleteSupportsOwnedMediaJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{deleteResult: true}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/public/video/cache/delete", strings.NewReader(`{"items":[{"source":"mediaJob","cache_key":"video_123"}]}`))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"deleted":1`) || videoGateway.deletedID != "video_123" {
		t.Fatalf("status=%d body=%s id=%q", recorder.Code, recorder.Body.String(), videoGateway.deletedID)
	}
}

func TestVideoRenameFallsBackToMigratedCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{renameError: gateway.ErrResponseNotFound}
	videoCache := &fakeLegacyVideoCache{}
	handler := NewHandler(Options{PublicEnabled: true, VideoCache: videoCache}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/public/video/rename", bytes.NewBufferString(`{"post_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","display_name":"Renamed local"}`))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || videoCache.renamedID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" || videoCache.renamedTo != "Renamed local" {
		t.Fatalf("status=%d body=%s id=%q title=%q", recorder.Code, recorder.Body.String(), videoCache.renamedID, videoCache.renamedTo)
	}
}

func TestVideoRenameDoesNotFallbackOnPersistentStoreFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{renameError: errors.New("database unavailable")}
	videoCache := &fakeLegacyVideoCache{}
	handler := NewHandler(Options{PublicEnabled: true, VideoCache: videoCache}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/public/video/rename", bytes.NewBufferString(`{"post_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","display_name":"Must fail"}`))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError || videoCache.renamedID != "" {
		t.Fatalf("status=%d body=%s cacheID=%q", recorder.Code, recorder.Body.String(), videoCache.renamedID)
	}
}

func TestVideoCacheListSurvivesLocalCacheFailureAndHugePage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{listed: []mediadomain.Job{{
		ID: "video-job-1", Status: mediadomain.StatusCompleted, UpstreamURL: "https://example.com/video.mp4", UpdatedAt: time.Now(),
	}}}
	videoCache := &fakeLegacyVideoCache{listError: errors.New("cache unavailable")}
	handler := NewHandler(Options{PublicEnabled: true, VideoCache: videoCache}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	page := strconv.Itoa(int(^uint(0) >> 1))
	request := httptest.NewRequest(http.MethodGet, "/v1/public/video/cache/list?page="+page+"&page_size=100", nil)
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"items":[]`) || !strings.Contains(recorder.Body.String(), `"total":1`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func (f *fakeLegacyVideoGateway) CancelVideo(_ context.Context, id string, _ clientkeydomain.Key) (mediadomain.Job, error) {
	f.cancelled = append(f.cancelled, id)
	return mediadomain.Job{ID: id, Status: mediadomain.StatusFailed, ErrorCode: "cancelled"}, nil
}

func (f *fakeLegacyVideoGateway) GenerateImage(context.Context, gateway.ImageGenerationInput) (*gateway.Result, error) {
	return nil, errors.New("not used")
}

func (f *fakeLegacyVideoGateway) CreateVideo(_ context.Context, input gateway.VideoInput) (mediadomain.Job, error) {
	if f.createError != nil {
		return mediadomain.Job{}, f.createError
	}
	f.created = append(f.created, input)
	id := "video-job-" + string(rune('0'+len(f.created)))
	return mediadomain.Job{ID: id, Status: mediadomain.StatusQueued, Progress: 0}, nil
}

func TestVideoStartStoresInlineReferencesBeforeCreatingPersistentJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{}
	references := &fakeVideoReferenceStore{}
	handler := NewHandler(Options{PublicEnabled: true, VideoReferenceStore: references}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	body := `{"prompt":"move","concurrent":2,"image_references":["data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/public/video/start", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || len(references.saved) != 1 || len(videoGateway.created) != 2 {
		t.Fatalf("status=%d body=%s saved=%d jobs=%d", recorder.Code, recorder.Body.String(), len(references.saved), len(videoGateway.created))
	}
	for _, input := range videoGateway.created {
		if len(input.ReferenceURLs) != 1 || input.ReferenceURLs[0] != "grok2api-media://image/stored-reference-1" {
			t.Fatalf("references=%v", input.ReferenceURLs)
		}
	}
}

func TestVideoStartStoresInlineReferencesIndependentlyOfPublicBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{}
	references := &fakeVideoReferenceStore{}
	handler := NewHandler(Options{PublicEnabled: true, VideoReferenceStore: references}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	inline := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	request := httptest.NewRequest(http.MethodPost, "/v1/public/video/start", bytes.NewBufferString(`{"prompt":"move","image_references":["`+inline+`"]}`))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || len(references.saved) != 1 || len(videoGateway.created) != 1 || videoGateway.created[0].ReferenceURLs[0] != "grok2api-media://image/stored-reference-1" {
		t.Fatalf("status=%d body=%s saved=%d references=%v", recorder.Code, recorder.Body.String(), len(references.saved), videoGateway.created)
	}
}

func TestVideoStartKeepsWebModelForFifteenSecondSingleReferenceGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/public/video/start", bytes.NewBufferString(`{
		"prompt":"move",
		"video_length":15,
		"source_image_urls":["https://example.com/reference.png"]
	}`))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || len(videoGateway.created) != 1 {
		t.Fatalf("status=%d body=%s jobs=%d", recorder.Code, recorder.Body.String(), len(videoGateway.created))
	}
	if input := videoGateway.created[0]; input.PublicModel != "grok-imagine-video" {
		t.Fatalf("model=%q want grok-imagine-video", input.PublicModel)
	}
}

func TestVideoStartKeepsBaseModelWhenVideo15IsNotRequired(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "ten second reference", body: `{"prompt":"move","video_length":10,"source_image_url":"https://example.com/reference.png"}`},
		{name: "fifteen second text", body: `{"prompt":"move","video_length":15}`},
		{name: "video extension", body: `{"prompt":"extend","video_length":15,"is_video_extension":true,"extend_post_id":"123e4567-e89b-12d3-a456-426614174000"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
			videoGateway := &fakeLegacyVideoGateway{}
			handler := NewHandler(Options{PublicEnabled: true}, authenticator, videoGateway)
			router := gin.New()
			handler.Register(router, nil, nil)

			request := httptest.NewRequest(http.MethodPost, "/v1/public/video/start", bytes.NewBufferString(test.body))
			request.Header.Set("Authorization", "Bearer g2-direct-key")
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK || len(videoGateway.created) != 1 {
				t.Fatalf("status=%d body=%s jobs=%d", recorder.Code, recorder.Body.String(), len(videoGateway.created))
			}
			if input := videoGateway.created[0]; input.PublicModel != "grok-imagine-video" {
				t.Fatalf("model=%q want grok-imagine-video", input.PublicModel)
			}
		})
	}
}

func TestVideoStartHidesPersistenceErrorsFromPublicResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{createError: errors.New("constraint failed: CHECK constraint failed: chk_media_jobs_input_json (275)")}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/public/video/start", bytes.NewBufferString(`{"prompt":"move"}`))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway || strings.Contains(strings.ToLower(recorder.Body.String()), "constraint") || !strings.Contains(recorder.Body.String(), "创建视频任务失败") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func (f *fakeLegacyVideoGateway) GetVideo(_ context.Context, id string, _ clientkeydomain.Key) (mediadomain.Job, error) {
	if f.polls == nil {
		f.polls = make(map[string]int)
	}
	f.polls[id]++
	if f.polls[id] == 1 {
		return mediadomain.Job{ID: id, Status: mediadomain.StatusInProgress, Progress: 35}, nil
	}
	return mediadomain.Job{ID: id, Status: mediadomain.StatusCompleted, Progress: 100, UpstreamURL: "https://example.com/video.mp4", Seconds: 6}, nil
}

func TestVideoStartAndSSEMapToPersistentGoJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{}
	handler := NewHandler(Options{PublicEnabled: true, VideoPollInterval: time.Millisecond}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	startBody := `{
		"prompt":"move",
		"aspect_ratio":"16:9",
		"video_length":6,
		"resolution_name":"720p",
		"preset":"spicy",
		"concurrent":2,
		"image_references":["data:image/png;base64,YWJj"]
	}`
	startRequest := httptest.NewRequest(http.MethodPost, "/v1/public/video/start", bytes.NewBufferString(startBody))
	startRequest.Header.Set("Authorization", "Bearer g2-direct-key")
	startRequest.Header.Set("Content-Type", "application/json")
	startRecorder := httptest.NewRecorder()
	router.ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	var startResponse struct {
		TaskID  string   `json:"task_id"`
		TaskIDs []string `json:"task_ids"`
	}
	if err := json.Unmarshal(startRecorder.Body.Bytes(), &startResponse); err != nil {
		t.Fatal(err)
	}
	if len(startResponse.TaskIDs) != 2 || startResponse.TaskID != startResponse.TaskIDs[0] || len(videoGateway.created) != 2 {
		t.Fatalf("response=%#v created=%d", startResponse, len(videoGateway.created))
	}
	for _, input := range videoGateway.created {
		if input.PublicModel != "grok-imagine-video" || input.Prompt != "move" || input.Duration != 6 || input.AspectRatio != "16:9" || input.Resolution != "720p" || input.Preset != "spicy" || len(input.ReferenceURLs) != 1 {
			t.Fatalf("input=%#v", input)
		}
	}

	sseRequest := httptest.NewRequest(http.MethodGet, "/v1/public/video/sse?task_id="+startResponse.TaskID+"&public_key=g2-direct-key", nil)
	sseRecorder := httptest.NewRecorder()
	router.ServeHTTP(sseRecorder, sseRequest)
	if sseRecorder.Code != http.StatusOK || !strings.HasPrefix(sseRecorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("sse status=%d content-type=%q body=%s", sseRecorder.Code, sseRecorder.Header().Get("Content-Type"), sseRecorder.Body.String())
	}
	for _, expected := range []string{"当前进度 35%", "[video](https://example.com/video.mp4)", `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(sseRecorder.Body.String(), expected) {
			t.Fatalf("SSE body missing %q: %s", expected, sseRecorder.Body.String())
		}
	}
}

func TestVideoExtensionMapsToNativeGoJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/public/video/start", bytes.NewBufferString(`{"prompt":"extend","is_video_extension":true,"source_task_id":"source-task-1","extend_post_id":"123e4567-e89b-12d3-a456-426614174000","video_extension_start_time":5.25,"original_post_id":"223e4567-e89b-12d3-a456-426614174000","file_attachment_id":"223e4567-e89b-12d3-a456-426614174000","stitch_with_extend":true}`))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(videoGateway.created) != 1 {
		t.Fatalf("jobs=%d", len(videoGateway.created))
	}
	input := videoGateway.created[0]
	if !input.IsExtension || input.SourceTaskID != "source-task-1" || input.ExtendPostID != "123e4567-e89b-12d3-a456-426614174000" || input.ExtensionStartTime != 5.25 || input.OriginalPostID != "223e4567-e89b-12d3-a456-426614174000" || !input.StitchWithExtend {
		t.Fatalf("input=%#v", input)
	}
}

func TestLastVideoPostIDUsesGeneratedPostInsteadOfUserID(t *testing.T) {
	value := "/v1/files/video/users-14610db1-ca48-4ba1-8509-c01304a4f508-generated-28455674-ccd9-4473-8b18-2fbeab04ae40-generated_video.mp4"
	if got := lastVideoPostID(value); got != "28455674-ccd9-4473-8b18-2fbeab04ae40" {
		t.Fatalf("post ID = %q", got)
	}
}

func TestVideoStopCancelsPersistentJobAndRemovesLegacyPollingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	startRequest := httptest.NewRequest(http.MethodPost, "/v1/public/video/start", bytes.NewBufferString(`{"prompt":"move"}`))
	startRequest.Header.Set("Authorization", "Bearer g2-direct-key")
	startRequest.Header.Set("Content-Type", "application/json")
	startRecorder := httptest.NewRecorder()
	router.ServeHTTP(startRecorder, startRequest)
	var startResponse struct {
		TaskID string `json:"task_id"`
	}
	_ = json.Unmarshal(startRecorder.Body.Bytes(), &startResponse)

	stopRequest := httptest.NewRequest(http.MethodPost, "/v1/public/video/stop", bytes.NewBufferString(`{"task_ids":["`+startResponse.TaskID+`"]}`))
	stopRequest.Header.Set("Authorization", "Bearer g2-direct-key")
	stopRequest.Header.Set("Content-Type", "application/json")
	stopRecorder := httptest.NewRecorder()
	router.ServeHTTP(stopRecorder, stopRequest)
	if stopRecorder.Code != http.StatusOK || !strings.Contains(stopRecorder.Body.String(), `"removed":1`) {
		t.Fatalf("stop status=%d body=%s", stopRecorder.Code, stopRecorder.Body.String())
	}
	if len(videoGateway.cancelled) != 1 || videoGateway.cancelled[0] != "video-job-1" {
		t.Fatalf("cancelled=%v", videoGateway.cancelled)
	}

	sseRequest := httptest.NewRequest(http.MethodGet, "/v1/public/video/sse?task_id="+startResponse.TaskID+"&public_key=g2-direct-key", nil)
	sseRecorder := httptest.NewRecorder()
	router.ServeHTTP(sseRecorder, sseRequest)
	if sseRecorder.Code != http.StatusNotFound {
		t.Fatalf("sse status=%d body=%s", sseRecorder.Code, sseRecorder.Body.String())
	}
}

func TestVideoCacheListAndRenameUsePersistentJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	videoGateway := &fakeLegacyVideoGateway{listed: []mediadomain.Job{{
		ID: "video-job-1", RequestID: "request-1", Status: mediadomain.StatusCompleted,
		UpstreamURL:  "https://example.com/123e4567-e89b-12d3-a456-426614174000.mp4",
		MetadataJSON: `{"display_name":"Saved title"}`, UpdatedAt: time.UnixMilli(123456),
	}}}
	videoCache := &fakeLegacyVideoCache{items: []LegacyCachedVideo{{
		Source: "legacy", CacheKey: "local.mp4",
		Name: "local.mp4", ViewURL: "/v1/files/video/local.mp4", PosterURL: "/v1/files/image/local.jpg", PostID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		DisplayName: "Migrated title", SizeBytes: 42, ModifiedAtMS: 234567,
	}}}
	handler := NewHandler(Options{PublicEnabled: true, VideoCache: videoCache}, authenticator, videoGateway)
	router := gin.New()
	handler.Register(router, nil, nil)

	listRequest := httptest.NewRequest(http.MethodGet, "/v1/public/video/cache/list?page=1&page_size=100", nil)
	listRequest.Header.Set("Authorization", "Bearer g2-direct-key")
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, listRequest)
	for _, expected := range []string{`"display_name":"Saved title"`, `"task_id":"request-1"`, `"post_id":"123e4567-e89b-12d3-a456-426614174000"`, `"view_url":"https://example.com/123e4567-e89b-12d3-a456-426614174000.mp4"`, `"display_name":"Migrated title"`, `"view_url":"/v1/files/video/local.mp4"`, `"source":"legacy"`, `"cache_key":"local.mp4"`, `"poster_url":"/v1/files/image/local.jpg"`, `"total":2`} {
		if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), expected) {
			t.Fatalf("list status=%d body=%s missing=%s", listRecorder.Code, listRecorder.Body.String(), expected)
		}
	}

	renameRequest := httptest.NewRequest(http.MethodPost, "/v1/public/video/rename", bytes.NewBufferString(`{"post_id":"123e4567-e89b-12d3-a456-426614174000","display_name":"New title"}`))
	renameRequest.Header.Set("Authorization", "Bearer g2-direct-key")
	renameRequest.Header.Set("Content-Type", "application/json")
	renameRecorder := httptest.NewRecorder()
	router.ServeHTTP(renameRecorder, renameRequest)
	if renameRecorder.Code != http.StatusOK || !strings.Contains(renameRecorder.Body.String(), `"status":"success"`) || videoGateway.renamedID != "123e4567-e89b-12d3-a456-426614174000" || videoGateway.renamedTo != "New title" {
		t.Fatalf("rename status=%d body=%s id=%q title=%q", renameRecorder.Code, renameRecorder.Body.String(), videoGateway.renamedID, videoGateway.renamedTo)
	}
}
