package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadinessEndpointReturnsStructuredDegradedStateAsReady(t *testing.T) {
	router := New(Dependencies{
		RequestTimeout: time.Second,
		MaxBodyBytes:   1024,
		Readiness: func(context.Context) ReadinessSnapshot {
			return ReadinessSnapshot{
				Ready: true, State: "degraded", UpdatedAt: time.Now().UTC(),
				Components: map[string]ReadinessComponent{
					"grok_build": {State: "ready"},
					"grok_web":   {State: "unavailable"},
				},
			}
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body ReadinessSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Ready || body.State != "degraded" || body.Components["grok_build"].State != "ready" {
		t.Fatalf("body = %#v", body)
	}
}

func TestReadinessEndpointReturns503WhileReconciling(t *testing.T) {
	router := New(Dependencies{RequestTimeout: time.Second, MaxBodyBytes: 1024, Readiness: func(context.Context) ReadinessSnapshot {
		return ReadinessSnapshot{Ready: false, State: "reconciling", UpdatedAt: time.Now().UTC()}
	}})
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"state":"reconciling"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInferenceTrafficIsRejectedWhileReconciling(t *testing.T) {
	router := New(Dependencies{RequestTimeout: time.Second, MaxBodyBytes: 1024, TrafficReady: func() bool { return false }})
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"code":"service_reconciling"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSystemInfoRequiresAdminAuthentication(t *testing.T) {
	router := New(Dependencies{RequestTimeout: time.Second, MaxBodyBytes: 1024, PublicAPIBaseURL: "https://api.example.com"})
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestFrontendStaticFilesAndSPAFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>app</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log('app')"), 0o600); err != nil {
		t.Fatal(err)
	}
	router := New(Dependencies{Logger: slog.Default(), RequestTimeout: time.Second, MaxBodyBytes: 1024, FrontendStaticPath: root})

	for _, test := range []struct {
		path        string
		status      int
		body        string
		cachePrefix string
	}{
		{path: "/gateway/assets/app.js", status: http.StatusOK, body: "console.log('app')", cachePrefix: "public"},
		{path: "/gateway/dashboard", status: http.StatusOK, body: "<html>app</html>", cachePrefix: "no-cache"},
		{path: "/gateway/assets/missing.js", status: http.StatusNotFound},
		{path: "/api/admin/v1/missing", status: http.StatusNotFound},
		{path: "/swagger/index.html", status: http.StatusNotFound},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			if test.body != "" && !strings.Contains(recorder.Body.String(), test.body) {
				t.Fatalf("body = %q", recorder.Body.String())
			}
			if test.cachePrefix != "" && !strings.HasPrefix(recorder.Header().Get("Cache-Control"), test.cachePrefix) {
				t.Fatalf("cache-control = %q", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestLegacyPagesAndGatewayFrontendAreServedByOneGoRouter(t *testing.T) {
	frontendRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(frontendRoot, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontendRoot, "index.html"), []byte("<html>gateway</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontendRoot, "assets", "app.js"), []byte("console.log('gateway')"), 0o600); err != nil {
		t.Fatal(err)
	}

	legacyRoot := t.TempDir()
	for _, directory := range []string{
		"public/pages",
		"public",
		"admin/pages",
		"common/img/favicon",
		"common/js",
	} {
		if err := os.MkdirAll(filepath.Join(legacyRoot, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"public/pages/login.html":             `<script src="/static/common/js/app.js?v=__ASSET_VERSION__"></script>`,
		"public/pages/chat.html":              "<html>chat</html>",
		"public/pages/imagine.html":           "<html>imagine</html>",
		"public/pages/imagine_workbench.html": "<html>workbench</html>",
		"public/pages/video.html":             "<html>video</html>",
		"public/pages/nsfw.html":              "<html>nsfw</html>",
		"public/pages/voice.html":             "<html>voice</html>",
		"admin/pages/login.html":              "<html>admin login</html>",
		"admin/pages/token.html":              "<html>admin token</html>",
		"admin/pages/config.html":             "<html>admin config</html>",
		"admin/pages/cache.html":              "<html>admin cache</html>",
		"public/manifest.webmanifest":         `{"name":"legacy"}`,
		"public/sw.js":                        "self.addEventListener('install', () => {})",
		"common/img/favicon/favicon.ico":      "icon",
		"common/js/app.js":                    "console.log('legacy')",
	}
	for relativePath, content := range files {
		if err := os.WriteFile(filepath.Join(legacyRoot, relativePath), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	router := New(Dependencies{
		Logger:              slog.Default(),
		RequestTimeout:      time.Second,
		MaxBodyBytes:        1024,
		FrontendStaticPath:  frontendRoot,
		LegacyStaticPath:    legacyRoot,
		LegacyAssetVersion:  "test-version",
		LegacyPublicEnabled: true,
	})

	tests := []struct {
		path        string
		status      int
		body        string
		location    string
		cache       string
		contentType string
	}{
		{path: "/", status: http.StatusTemporaryRedirect, location: "/login"},
		{path: "/login", status: http.StatusOK, body: "v=test-version", cache: "no-store"},
		{path: "/chat", status: http.StatusOK, body: "<html>chat</html>", cache: "no-store"},
		{path: "/admin", status: http.StatusTemporaryRedirect, location: "/admin/login"},
		{path: "/admin/token", status: http.StatusOK, body: "admin token", cache: "no-store"},
		{path: "/static/common/js/app.js", status: http.StatusOK, body: "legacy", cache: "no-cache"},
		{path: "/manifest.webmanifest", status: http.StatusOK, body: "legacy", contentType: "application/manifest+json"},
		{path: "/sw.js", status: http.StatusOK, body: "install", contentType: "application/javascript"},
		{path: "/favicon.ico", status: http.StatusOK, body: "icon", contentType: "image/x-icon"},
		{path: "/gateway/assets/app.js", status: http.StatusOK, body: "gateway", cache: "public"},
		{path: "/gateway/dashboard", status: http.StatusOK, body: "<html>gateway</html>", cache: "no-cache"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			if test.body != "" && !strings.Contains(recorder.Body.String(), test.body) {
				t.Fatalf("body = %q, want substring %q", recorder.Body.String(), test.body)
			}
			if test.location != "" && recorder.Header().Get("Location") != test.location {
				t.Fatalf("location = %q, want %q", recorder.Header().Get("Location"), test.location)
			}
			if test.cache != "" && !strings.HasPrefix(recorder.Header().Get("Cache-Control"), test.cache) {
				t.Fatalf("cache-control = %q, want prefix %q", recorder.Header().Get("Cache-Control"), test.cache)
			}
			if test.contentType != "" && !strings.HasPrefix(recorder.Header().Get("Content-Type"), test.contentType) {
				t.Fatalf("content-type = %q, want prefix %q", recorder.Header().Get("Content-Type"), test.contentType)
			}
		})
	}
}

func TestSwaggerRegistrationFollowsStartupConfig(t *testing.T) {
	disabled := New(Dependencies{Logger: slog.Default(), RequestTimeout: time.Second, MaxBodyBytes: 1024})
	disabledRequest := httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
	disabledRecorder := httptest.NewRecorder()
	disabled.ServeHTTP(disabledRecorder, disabledRequest)
	if disabledRecorder.Code != http.StatusNotFound {
		t.Fatalf("disabled swagger status = %d, want %d", disabledRecorder.Code, http.StatusNotFound)
	}

	enabled := New(Dependencies{Logger: slog.Default(), RequestTimeout: time.Second, MaxBodyBytes: 1024, SwaggerEnabled: true})
	enabledRequest := httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
	enabledRecorder := httptest.NewRecorder()
	enabled.ServeHTTP(enabledRecorder, enabledRequest)
	if enabledRecorder.Code != http.StatusOK {
		t.Fatalf("enabled swagger status = %d, want %d", enabledRecorder.Code, http.StatusOK)
	}
	var document struct {
		Info struct {
			Title string `json:"title"`
		} `json:"info"`
	}
	if err := json.Unmarshal(enabledRecorder.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode swagger document: %v", err)
	}
	if document.Info.Title != "Grok2API" {
		t.Fatalf("swagger title = %q, want %q", document.Info.Title, "Grok2API")
	}
}
