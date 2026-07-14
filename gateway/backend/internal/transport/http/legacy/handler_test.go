package legacy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type fakeClientAuthenticator struct {
	wantRaw  string
	releases int
}

func (f *fakeClientAuthenticator) Authenticate(_ context.Context, raw string) (clientkeydomain.Key, func(), error) {
	if raw != f.wantRaw {
		return clientkeydomain.Key{}, nil, errInvalidTestKey
	}
	return clientkeydomain.Key{ID: 7, Name: "legacy-page", Enabled: true, RPMLimit: 120, MaxConcurrent: 8}, func() {
		f.releases++
	}, nil
}

type testKeyError string

func (e testKeyError) Error() string { return string(e) }

const errInvalidTestKey = testKeyError("invalid test key")

func TestPublicRoutesMapLegacyKeyToPersistentClientKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-client-secret"}
	handler := NewHandler(Options{
		PublicEnabled: true,
		PublicKey:     "legacy-public-key",
		ClientKey:     "g2-client-secret",
		StorageType:   "sqlite",
	}, authenticator)
	router := gin.New()
	handler.Register(router, func(group *gin.RouterGroup) {
		group.GET("/identity", func(c *gin.Context) {
			value, ok := c.Get(middleware.ClientKey)
			if !ok {
				c.Status(http.StatusInternalServerError)
				return
			}
			key := value.(clientkeydomain.Key)
			c.JSON(http.StatusOK, gin.H{"id": key.ID})
		})
	}, nil)

	for _, route := range []string{"/v1/public/verify", "/v1/public/identity"} {
		request := httptest.NewRequest(http.MethodGet, route, nil)
		request.Header.Set("Authorization", "Bearer legacy-public-key")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", route, recorder.Code, recorder.Body.String())
		}
	}
	if authenticator.releases != 2 {
		t.Fatalf("releases = %d, want 2", authenticator.releases)
	}
}

func TestPublicRoutesAcceptEventSourceQueryCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-client-secret"}
	handler := NewHandler(Options{
		PublicEnabled: true,
		PublicKey:     "legacy-public-key",
		ClientKey:     "g2-client-secret",
	}, authenticator)
	router := gin.New()
	handler.Register(router, func(group *gin.RouterGroup) {
		group.GET("/events", func(c *gin.Context) { c.Status(http.StatusOK) })
	}, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/public/events?public_key=legacy-public-key", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicRoutesAcceptConfiguredClientKeyDirectly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	handler := NewHandler(Options{PublicEnabled: true, StorageType: "sqlite"}, authenticator)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/public/verify", nil)
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicRoutesAllowEmptyPageKeyWhenClientKeyIsConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-client-secret"}
	handler := NewHandler(Options{PublicEnabled: true, ClientKey: "g2-client-secret", StorageType: "sqlite"}, authenticator)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/public/verify", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLegacyAdminVerifyAndStorageUseConstantCredentialBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(Options{AdminKey: "legacy-admin-key", StorageType: "sqlite"}, nil)
	router := gin.New()
	handler.Register(router, nil, nil)

	for _, test := range []struct {
		name   string
		path   string
		key    string
		status int
		body   string
	}{
		{name: "missing", path: "/v1/admin/verify", status: http.StatusUnauthorized},
		{name: "wrong", path: "/v1/admin/verify", key: "wrong", status: http.StatusUnauthorized},
		{name: "verify", path: "/v1/admin/verify", key: "legacy-admin-key", status: http.StatusOK},
		{name: "storage", path: "/v1/admin/storage", key: "legacy-admin-key", status: http.StatusOK, body: `"type":"sqlite"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.key != "" {
				request.Header.Set("Authorization", "Bearer "+test.key)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			if test.body != "" && !strings.Contains(recorder.Body.String(), test.body) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
		})
	}
}

func TestDisabledPublicCompatibilityReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(Options{PublicEnabled: false}, nil)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/public/verify", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
