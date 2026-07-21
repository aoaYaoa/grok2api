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

	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
)

func testDependencies() Dependencies {
	return Dependencies{RequestTimeout: time.Second, MaxBodyBytes: 1024, ConcurrencyGate: middleware.NewConcurrencyGate(1024)}
}

func TestReadinessEndpointReturnsStructuredDegradedStateAsReady(t *testing.T) {
	deps := testDependencies()
	deps.Readiness = func(context.Context) ReadinessSnapshot {
		return ReadinessSnapshot{
			Ready: true, State: "degraded", UpdatedAt: time.Now().UTC(),
			Components: map[string]ReadinessComponent{
				"grok_build": {State: "ready"},
				"grok_web":   {State: "unavailable"},
			},
		}
	}
	router := New(deps)
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
	deps := testDependencies()
	deps.Readiness = func(context.Context) ReadinessSnapshot {
		return ReadinessSnapshot{Ready: false, State: "reconciling", UpdatedAt: time.Now().UTC()}
	}
	router := New(deps)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"state":"reconciling"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInferenceTrafficIsRejectedWhileReconciling(t *testing.T) {
	deps := testDependencies()
	deps.TrafficReady = func() bool { return false }
	router := New(deps)
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"code":"service_reconciling"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSystemEndpointsRequireAdminAuthentication(t *testing.T) {
	deps := testDependencies()
	deps.PublicAPIBaseURL = "https://api.example.com"
	router := New(deps)
	for _, route := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/admin/v1/system"},
		{method: http.MethodGet, path: "/api/admin/v1/system/version"},
		{method: http.MethodPost, path: "/api/admin/v1/system/update/check"},
	} {
		request := httptest.NewRequest(route.method, route.path, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
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
	deps := testDependencies()
	deps.Logger = slog.Default()
	deps.FrontendStaticPath = root
	router := New(deps)

	for _, test := range []struct {
		path        string
		status      int
		body        string
		cachePrefix string
	}{
		{path: "/gateway/assets/app.js", status: http.StatusOK, body: "console.log('app')", cachePrefix: "public"},
		{path: "/gateway/dashboard", status: http.StatusOK, body: "<html>app</html>", cachePrefix: "no-cache"},
		{path: "/gateway/assets/missing.js", status: http.StatusOK, body: "location.reload()", cachePrefix: "no-store"},
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
	request := httptest.NewRequest(http.MethodGet, "/gateway/assets/missing.js", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/javascript") {
		t.Fatalf("content-type = %q", contentType)
	}
}

func TestPublicAndGatewayReactFrontendsAreServedByOneGoRouter(t *testing.T) {
	frontendRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(frontendRoot, "admin", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(frontendRoot, "public", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontendRoot, "admin", "index.html"), []byte("<html>gateway-react</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"admin/assets/app.js":         "console.log('gateway')",
		"public/index.html":           "<html>public-react</html>",
		"public/assets/public.js":     "console.log('public-react')",
		"public/manifest.webmanifest": `{"name":"react"}`,
		"public/sw.js":                "self.registration.unregister()",
		"public/grok2api.png":         "icon",
		"public/favicon.ico":          "icon",
	}
	for relativePath, content := range files {
		if err := os.WriteFile(filepath.Join(frontendRoot, relativePath), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	legacyCacheRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(legacyCacheRoot, "image"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyCacheRoot, "image", "cached.png"), []byte("cached-image"), 0o600); err != nil {
		t.Fatal(err)
	}

	router := New(Dependencies{
		Logger:              slog.Default(),
		RequestTimeout:      time.Second,
		MaxBodyBytes:        1024,
		ConcurrencyGate:     middleware.NewConcurrencyGate(1024),
		FrontendStaticPath:  frontendRoot,
		LegacyCachePath:     legacyCacheRoot,
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
		{path: "/login", status: http.StatusOK, body: "public-react", cache: "no-cache"},
		{path: "/chat", status: http.StatusOK, body: "public-react", cache: "no-cache"},
		{path: "/admin", status: http.StatusTemporaryRedirect, location: "/gateway/dashboard"},
		{path: "/admin/login", status: http.StatusTemporaryRedirect, location: "/gateway/login"},
		{path: "/admin/token", status: http.StatusTemporaryRedirect, location: "/gateway/accounts"},
		{path: "/admin/config", status: http.StatusTemporaryRedirect, location: "/gateway/settings"},
		{path: "/admin/cache", status: http.StatusTemporaryRedirect, location: "/gateway/cache"},
		{path: "/admin/anything/deep", status: http.StatusTemporaryRedirect, location: "/gateway/dashboard"},
		{path: "/static/common/js/app.js", status: http.StatusNotFound},
		{path: "/assets/public.js", status: http.StatusOK, body: "public-react", cache: "public"},
		{path: "/manifest.webmanifest", status: http.StatusOK, body: "react"},
		{path: "/sw.js", status: http.StatusOK, body: "registration.unregister", cache: "no-store", contentType: "text/javascript"},
		{path: "/favicon.ico", status: http.StatusOK, body: "icon", contentType: "image/x-icon"},
		{path: "/v1/files/image/cached.png", status: http.StatusOK, body: "cached-image", contentType: "image/png"},
		{path: "/gateway/assets/app.js", status: http.StatusOK, body: "gateway", cache: "public"},
		{path: "/gateway/dashboard", status: http.StatusOK, body: "gateway-react", cache: "no-cache"},
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
	disabledDeps := testDependencies()
	disabledDeps.Logger = slog.Default()
	disabled := New(disabledDeps)
	disabledRequest := httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
	disabledRecorder := httptest.NewRecorder()
	disabled.ServeHTTP(disabledRecorder, disabledRequest)
	if disabledRecorder.Code != http.StatusNotFound {
		t.Fatalf("disabled swagger status = %d, want %d", disabledRecorder.Code, http.StatusNotFound)
	}

	enabledDeps := testDependencies()
	enabledDeps.Logger = slog.Default()
	enabledDeps.SwaggerEnabled = true
	enabled := New(enabledDeps)
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
