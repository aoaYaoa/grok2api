package cache

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerListsServesRenamesAndDeletesLegacyCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	for _, dir := range []string{"image", "video", "media-meta"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	imageName := "preview-x0.png"
	videoName := "generated-123e4567-e89b-12d3-a456-426614174000-output.mp4"
	if err := os.WriteFile(filepath.Join(root, "image", imageName), []byte("image-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image", "users-u-preview-x0.png"), []byte("nested-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "video", videoName), []byte("video-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := `{"media_type":"video","post_id":"123e4567-e89b-12d3-a456-426614174000","share_link":"https://grok.com/imagine/post/123e4567-e89b-12d3-a456-426614174000","display_name":"Before"}`
	if err := os.WriteFile(filepath.Join(root, "media-meta", "123e4567-e89b-12d3-a456-426614174000.json"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}

	handler, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler.RegisterPublic(router)
	handler.RegisterAdmin(router.Group("/api/admin/v1"))
	handler.RegisterLegacy(router.Group("/v1/admin"))

	stats := performRequest(router, http.MethodGet, "/api/admin/v1/cache", "")
	if stats.Code != http.StatusOK || !strings.Contains(stats.Body.String(), `"data":{"image":{"count":2`) || !strings.Contains(stats.Body.String(), `"video":{"count":1`) {
		t.Fatalf("stats status=%d body=%s", stats.Code, stats.Body.String())
	}

	list := performRequest(router, http.MethodGet, "/api/admin/v1/cache/items?type=video&page=1&pageSize=10", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"displayName":"Before"`) || !strings.Contains(list.Body.String(), `"viewURL":"/v1/files/video/`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	served := performRequest(router, http.MethodGet, "/v1/files/image/"+imageName, "")
	if served.Code != http.StatusOK || served.Body.String() != "image-data" {
		t.Fatalf("served status=%d body=%q", served.Code, served.Body.String())
	}
	nested := performRequest(router, http.MethodGet, "/v1/files/image/users/u/preview-x0.png", "")
	if nested.Code != http.StatusOK || nested.Body.String() != "nested-image" {
		t.Fatalf("nested status=%d body=%q", nested.Code, nested.Body.String())
	}

	rename := performRequest(router, http.MethodPatch, "/api/admin/v1/cache/videos/123e4567-e89b-12d3-a456-426614174000", `{"displayName":"After"}`)
	if rename.Code != http.StatusOK || !strings.Contains(rename.Body.String(), `"displayName":"After"`) {
		t.Fatalf("rename status=%d body=%s", rename.Code, rename.Body.String())
	}
	updated, err := os.ReadFile(filepath.Join(root, "media-meta", "123e4567-e89b-12d3-a456-426614174000.json"))
	if err != nil || !strings.Contains(string(updated), `"display_name": "After"`) {
		t.Fatalf("updated metadata=%s err=%v", updated, err)
	}
	tooLong := performRequest(router, http.MethodPatch, "/api/admin/v1/cache/videos/123e4567-e89b-12d3-a456-426614174000", `{"displayName":"`+strings.Repeat("a", 161)+`"}`)
	if tooLong.Code != http.StatusBadRequest {
		t.Fatalf("long rename status=%d body=%s", tooLong.Code, tooLong.Body.String())
	}

	deleted := performRequest(router, http.MethodDelete, "/api/admin/v1/cache/items?type=image&name="+imageName, "")
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"deleted":true`) {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}

	legacy := performRequest(router, http.MethodGet, "/v1/admin/cache/list?type=video&page=1&page_size=10", "")
	if legacy.Code != http.StatusOK || !strings.Contains(legacy.Body.String(), `"status":"success"`) || !strings.Contains(legacy.Body.String(), `"display_name":"After"`) {
		t.Fatalf("legacy status=%d body=%s", legacy.Code, legacy.Body.String())
	}
}

func TestHandlerRejectsTraversal(t *testing.T) {
	handler, err := NewHandler(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler.RegisterPublic(router)
	response := performRequest(router, http.MethodGet, "/v1/files/image/..%2Fsecret", "")
	if response.Code != http.StatusBadRequest && response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerRejectsSymlinkedPublicFiles(t *testing.T) {
	root := t.TempDir()
	handler, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "image", "linked.png")); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler.RegisterPublic(router)
	response := performRequest(router, http.MethodGet, "/v1/files/image/linked.png", "")
	if response.Code != http.StatusBadRequest && response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestNewHandlerRejectsSymlinkedMediaDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "image")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHandler(root); err == nil {
		t.Fatal("symlinked image directory was accepted")
	}
}

func TestAdminErrorsUseTheGatewayEnvelope(t *testing.T) {
	handler, err := NewHandler(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler.RegisterAdmin(router.Group("/api/admin/v1"))
	response := performRequest(router, http.MethodGet, "/api/admin/v1/cache/items?type=file", "")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"error":{"code":"invalidCacheRequest"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	invalidJSON := performRequest(router, http.MethodPatch, "/api/admin/v1/cache/videos/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", `{`)
	if invalidJSON.Code != http.StatusBadRequest || !strings.Contains(invalidJSON.Body.String(), `"error":{"code":"invalidCacheRequest"`) {
		t.Fatalf("invalid json status=%d body=%s", invalidJSON.Code, invalidJSON.Body.String())
	}
}

func TestHandlerRejectsMissingVideoRenameAndHugePage(t *testing.T) {
	handler, err := NewHandler(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler.RegisterAdmin(router.Group("/api/admin/v1"))
	rename := performRequest(router, http.MethodPatch, "/api/admin/v1/cache/videos/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", `{"displayName":"orphan"}`)
	if rename.Code != http.StatusBadRequest {
		t.Fatalf("rename status=%d body=%s", rename.Code, rename.Body.String())
	}
	page := strconv.Itoa(int(^uint(0) >> 1))
	list := performRequest(router, http.MethodGet, "/api/admin/v1/cache/items?type=image&page="+page+"&pageSize=1000", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"items":[]`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
}

func TestEmptyStatsReportZeroMegabytes(t *testing.T) {
	handler, err := NewHandler(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler.RegisterAdmin(router.Group("/api/admin/v1"))
	response := performRequest(router, http.MethodGet, "/api/admin/v1/cache", "")
	var envelope struct {
		Data struct {
			Image Stats `json:"image"`
			Video Stats `json:"video"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Image.SizeMB != 0 || envelope.Data.Video.SizeMB != 0 {
		t.Fatalf("empty sizeMB image=%v video=%v", envelope.Data.Image.SizeMB, envelope.Data.Video.SizeMB)
	}
}

func TestHandlerDeleteItemCleansVideoMetadataOnlyAfterLastMatchingVideo(t *testing.T) {
	root := t.TempDir()
	postID := "123e4567-e89b-12d3-a456-426614174000"
	firstVideo := "first-" + postID + ".mp4"
	secondVideo := "second-" + postID + ".mp4"
	metadataPath := filepath.Join(root, "media-meta", postID+".json")
	handler, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "video", firstVideo), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "video", secondVideo), []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, []byte(`{"media_type":"video","post_id":"`+postID+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if deleted, err := handler.DeleteItem("video", firstVideo); err != nil || !deleted {
		t.Fatalf("first delete: deleted=%v err=%v", deleted, err)
	}
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("metadata removed while another matching video remains: %v", err)
	}
	if deleted, err := handler.DeleteItem("video", secondVideo); err != nil || !deleted {
		t.Fatalf("second delete: deleted=%v err=%v", deleted, err)
	}
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Fatalf("metadata still exists after last matching video: err=%v", err)
	}

	if _, err := handler.DeleteItem("video", "../"+postID+".mp4"); err == nil {
		t.Fatal("traversal delete was accepted")
	}
}

func TestHandlerDeleteItemRejectsSymlinkedFile(t *testing.T) {
	root := t.TempDir()
	handler, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "secret.mp4")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "video", "linked.mp4")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}

	if deleted, err := handler.DeleteItem("video", "linked.mp4"); err == nil || deleted {
		t.Fatalf("symlink delete accepted: deleted=%v err=%v", deleted, err)
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("symlink target was removed: %v", err)
	}
}

func TestHandlerDeleteItemRejectsNormalizedPathThatTargetsAnotherFile(t *testing.T) {
	root := t.TempDir()
	handler, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "video", "foo-bar.mp4")
	if err := os.WriteFile(target, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	if deleted, err := handler.DeleteItem("video", "foo/bar.mp4"); err == nil || deleted {
		t.Fatalf("normalized path was accepted: deleted=%v err=%v", deleted, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("normalized path deleted another file: %v", err)
	}
}

func TestHandlerDeleteItemKeepsVideoWhenMetadataCleanupFails(t *testing.T) {
	root := t.TempDir()
	postID := "123e4567-e89b-12d3-a456-426614174000"
	name := "generated-" + postID + "-output.mp4"
	videoPath := filepath.Join(root, "video", name)
	metadataPath := filepath.Join(root, "media-meta", postID+".json")
	handler, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(videoPath, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(metadataPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataPath, "blocked"), []byte("metadata"), 0o600); err != nil {
		t.Fatal(err)
	}

	if deleted, err := handler.DeleteItem("video", name); err == nil || deleted {
		t.Fatalf("metadata cleanup failure was hidden: deleted=%v err=%v", deleted, err)
	}
	if _, err := os.Stat(videoPath); err != nil {
		t.Fatalf("video was removed before metadata cleanup succeeded: %v", err)
	}
	if err := os.RemoveAll(metadataPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, []byte(`{"media_type":"video","post_id":"`+postID+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if deleted, err := handler.DeleteItem("video", name); err != nil || !deleted {
		t.Fatalf("retry delete: deleted=%v err=%v", deleted, err)
	}
	if _, err := os.Stat(videoPath); !os.IsNotExist(err) {
		t.Fatalf("video still exists after retry: %v", err)
	}
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Fatalf("metadata still exists after retry: %v", err)
	}
}

func TestHandlerDeleteItemUsesHeldRootsAfterDirectoriesAreReplaced(t *testing.T) {
	root := t.TempDir()
	postID := "123e4567-e89b-12d3-a456-426614174000"
	name := "generated-" + postID + "-output.mp4"
	handler, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "video", name), []byte("original video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "media-meta", postID+".json"), []byte("original metadata"), 0o600); err != nil {
		t.Fatal(err)
	}

	heldVideoDir := filepath.Join(root, "video-held")
	heldMetadataDir := filepath.Join(root, "media-meta-held")
	if err := os.Rename(filepath.Join(root, "video"), heldVideoDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "media-meta"), heldMetadataDir); err != nil {
		t.Fatal(err)
	}
	externalVideoDir := t.TempDir()
	externalMetadataDir := t.TempDir()
	externalMetadataPath := filepath.Join(externalMetadataDir, postID+".json")
	if err := os.WriteFile(externalMetadataPath, []byte("outside metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalVideoDir, filepath.Join(root, "video")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalMetadataDir, filepath.Join(root, "media-meta")); err != nil {
		t.Fatal(err)
	}

	if deleted, err := handler.DeleteItem("video", name); err != nil || !deleted {
		t.Fatalf("delete through held roots: deleted=%v err=%v", deleted, err)
	}
	if _, err := os.Stat(filepath.Join(heldVideoDir, name)); !os.IsNotExist(err) {
		t.Fatalf("original video still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(heldMetadataDir, postID+".json")); !os.IsNotExist(err) {
		t.Fatalf("original metadata still exists: %v", err)
	}
	if raw, err := os.ReadFile(externalMetadataPath); err != nil || string(raw) != "outside metadata" {
		t.Fatalf("external metadata was changed: raw=%q err=%v", raw, err)
	}
}

func performRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
