package httpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublicFrontendOwnsEveryPublicWorkspaceRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(`<div id="root">public-react</div>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte(`react`), 0o600); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	registerPublicFrontend(router, root, true)

	for _, route := range []string{"/login", "/chat", "/imagine", "/imagine-workbench", "/video", "/nsfw", "/voice"} {
		request := httptest.NewRequest(http.MethodGet, route, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "public-react") {
			t.Fatalf("%s status=%d body=%s", route, recorder.Code, recorder.Body.String())
		}
	}
	assetRequest := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	assetRecorder := httptest.NewRecorder()
	router.ServeHTTP(assetRecorder, assetRequest)
	if assetRecorder.Code != http.StatusOK || assetRecorder.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset status=%d cache=%q", assetRecorder.Code, assetRecorder.Header().Get("Cache-Control"))
	}
}

func TestPublicFrontendDisabledRedirectsRootToAdminAndHidesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPublicFrontend(router, t.TempDir(), false)
	rootRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRecorder := httptest.NewRecorder()
	router.ServeHTTP(rootRecorder, rootRequest)
	if rootRecorder.Code != http.StatusTemporaryRedirect || rootRecorder.Header().Get("Location") != "/gateway/login" {
		t.Fatalf("root status=%d location=%q", rootRecorder.Code, rootRecorder.Header().Get("Location"))
	}
}
