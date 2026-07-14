package legacy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
	"github.com/gin-gonic/gin"
)

type fakeLegacyAccountService struct {
	views       []accountapp.View
	updated     map[uint64]accountapp.UpdateInput
	deleted     []uint64
	importedRaw string
	refreshed   []uint64
}

func (f *fakeLegacyAccountService) List(_ context.Context, page, pageSize int, _ string, _ accountapp.ListFilter) ([]accountapp.View, int64, error) {
	start := (page - 1) * pageSize
	if start >= len(f.views) {
		return nil, int64(len(f.views)), nil
	}
	end := min(len(f.views), start+pageSize)
	return append([]accountapp.View(nil), f.views[start:end]...), int64(len(f.views)), nil
}

func (f *fakeLegacyAccountService) ImportWebCredentials(_ context.Context, data []byte) (accountapp.ImportResult, error) {
	f.importedRaw = string(data)
	result := accountapp.ImportResult{}
	for _, line := range strings.Split(string(data), "\n") {
		token := canonicalSSO(line)
		if token == "" {
			continue
		}
		sourceKey := "sso:" + security.HashToken(token)
		found := false
		for _, view := range f.views {
			if view.Credential.SourceKey == sourceKey {
				found = true
				break
			}
		}
		if found {
			continue
		}
		id := uint64(len(f.views) + 1)
		f.views = append(f.views, accountapp.View{Credential: accountdomain.Credential{
			ID: id, Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO,
			SourceKey: sourceKey, Name: "imported", Enabled: true, AuthStatus: accountdomain.AuthStatusActive,
		}})
		result.Created++
		result.AccountIDs = append(result.AccountIDs, id)
	}
	return result, nil
}

func (f *fakeLegacyAccountService) Update(_ context.Context, id uint64, input accountapp.UpdateInput) (accountapp.View, error) {
	if f.updated == nil {
		f.updated = make(map[uint64]accountapp.UpdateInput)
	}
	f.updated[id] = input
	for index := range f.views {
		if f.views[index].Credential.ID == id {
			if input.Enabled != nil {
				f.views[index].Credential.Enabled = *input.Enabled
			}
			if input.Name != nil {
				f.views[index].Credential.Name = *input.Name
			}
			return f.views[index], nil
		}
	}
	return accountapp.View{}, nil
}

func (f *fakeLegacyAccountService) Delete(_ context.Context, id uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeLegacyAccountService) RefreshWebQuota(_ context.Context, id uint64) ([]accountdomain.QuotaWindow, error) {
	f.refreshed = append(f.refreshed, id)
	return []accountdomain.QuotaWindow{{AccountID: id, Mode: "auto", Remaining: 90, Total: 100}}, nil
}

func (f *fakeLegacyAccountService) SyncWebQuotaAccountsWithProgress(ctx context.Context, ids []uint64, progress accountapp.BatchProgressObserver) (int, int, error) {
	succeeded := 0
	failed := 0
	if progress != nil {
		if err := progress(0, len(ids)); err != nil {
			return 0, 0, err
		}
	}
	for index, id := range ids {
		if _, err := f.RefreshWebQuota(ctx, id); err != nil {
			failed++
		} else {
			succeeded++
		}
		if progress != nil {
			if err := progress(index+1, len(ids)); err != nil {
				return succeeded, failed, err
			}
		}
	}
	return succeeded, failed, nil
}

func TestLegacyTokenListUsesOpaqueHandlesAndGoQuotaWindows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	synced := now.Add(-time.Minute)
	accounts := &fakeLegacyAccountService{views: []accountapp.View{
		{Credential: accountdomain.Credential{ID: 1, Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, SourceKey: "sso:secret-hash", WebTier: accountdomain.WebTierSuper, Name: "super", Enabled: true, AuthStatus: accountdomain.AuthStatusActive, CreatedAt: now}, QuotaWindows: []accountdomain.QuotaWindow{
			{Mode: "weekly", Remaining: 0, Total: 10000, UsagePercent: 100, Breakdown: []accountdomain.QuotaBreakdown{{ProductCode: accountdomain.QuotaProductChat, UsagePercent: 0}, {ProductCode: accountdomain.QuotaProductImagine, UsagePercent: 100}}, SyncedAt: &synced, Source: accountdomain.QuotaSourceUpstream},
			{Mode: "auto", Remaining: 85, Total: 100, WindowSeconds: 7200, SyncedAt: &synced, Source: accountdomain.QuotaSourceUpstream},
			{Mode: "fast", Remaining: 139, Total: 140, WindowSeconds: 7200, SyncedAt: &synced, Source: accountdomain.QuotaSourceUpstream},
		}},
		{Credential: accountdomain.Credential{ID: 2, Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, SourceKey: "sso:other-hash", WebTier: accountdomain.WebTierBasic, Name: "basic", Enabled: false, AuthStatus: accountdomain.AuthStatusActive, CreatedAt: now}},
	}}
	handler := NewHandler(Options{AdminKey: "admin", Accounts: accounts}, nil)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/admin/tokens", nil)
	request.Header.Set("Authorization", "Bearer admin")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"ssoSuper"`, `"token":"account:1"`, `"status":"active"`, `"weekly":{"remaining":0`, `"breakdown":[{"product_code":4,"usage_percent":0},{"product_code":5,"usage_percent":100}]`, `"auto":{"remaining":85`, `"fast":{"remaining":139`, `"ssoBasic"`, `"status":"disabled"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("body missing %q: %s", expected, body)
		}
	}
	if strings.Contains(body, "secret-hash") || strings.Contains(body, `"heavy"`) {
		t.Fatalf("response exposed a source key or invented quota: %s", body)
	}
}

func TestLegacyTokenSaveReconcilesHandlesAndNewWebImports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accounts := &fakeLegacyAccountService{views: []accountapp.View{
		{Credential: accountdomain.Credential{ID: 1, Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, SourceKey: "sso:" + security.HashToken("keep"), Name: "keep", Enabled: true, AuthStatus: accountdomain.AuthStatusActive}},
		{Credential: accountdomain.Credential{ID: 2, Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, SourceKey: "sso:" + security.HashToken("remove"), Name: "remove", Enabled: true, AuthStatus: accountdomain.AuthStatusActive}},
	}}
	handler := NewHandler(Options{AdminKey: "admin", Accounts: accounts}, nil)
	router := gin.New()
	handler.Register(router, nil, nil)

	payload := `{"ssoBasic":[{"token":"account:1","status":"disabled","note":"kept note"},{"token":"new-token","status":"active","note":"new note"}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/tokens", bytes.NewBufferString(payload))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(accounts.importedRaw, "new-token") {
		t.Fatalf("imported=%q", accounts.importedRaw)
	}
	if update, ok := accounts.updated[1]; !ok || update.Enabled == nil || *update.Enabled || update.Name == nil || *update.Name != "kept note" {
		t.Fatalf("updated=%#v", accounts.updated)
	}
	if update, ok := accounts.updated[3]; !ok || update.Enabled == nil || !*update.Enabled || update.Name == nil || *update.Name != "new note" {
		t.Fatalf("new account update=%#v", accounts.updated)
	}
	if len(accounts.deleted) != 1 || accounts.deleted[0] != 2 {
		t.Fatalf("deleted=%#v", accounts.deleted)
	}
}

func TestLegacySingleTokenRefreshUsesOpaqueAccountHandle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accounts := &fakeLegacyAccountService{views: []accountapp.View{{Credential: accountdomain.Credential{ID: 1, Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, SourceKey: "sso:hash", Enabled: true, AuthStatus: accountdomain.AuthStatusActive}}}}
	handler := NewHandler(Options{AdminKey: "admin", Accounts: accounts}, nil)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/admin/tokens/refresh", bytes.NewBufferString(`{"token":"account:1"}`))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"account:1":true`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(accounts.refreshed) != 1 || accounts.refreshed[0] != 1 {
		t.Fatalf("refreshed=%#v", accounts.refreshed)
	}
}
