package legacy

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
	"github.com/gin-gonic/gin"
)

type LegacyAccountService interface {
	List(context.Context, int, int, string, accountapp.ListFilter) ([]accountapp.View, int64, error)
	ImportWebCredentials(context.Context, []byte) (accountapp.ImportResult, error)
	Update(context.Context, uint64, accountapp.UpdateInput) (accountapp.View, error)
	Delete(context.Context, uint64) error
	RefreshWebQuota(context.Context, uint64) ([]accountdomain.QuotaWindow, error)
	SyncWebQuotaAccountsWithProgress(context.Context, []uint64, accountapp.BatchProgressObserver) (int, int, error)
}

type legacyTokenEntry struct {
	Token          string                      `json:"token"`
	Status         string                      `json:"status"`
	Quota          map[string]legacyQuotaValue `json:"quota"`
	Note           string                      `json:"note"`
	FailCount      int                         `json:"fail_count"`
	UseCount       int                         `json:"use_count"`
	Tags           []string                    `json:"tags"`
	CreatedAt      int64                       `json:"created_at,omitempty"`
	LastUsedAt     int64                       `json:"last_used_at,omitempty"`
	LastFailReason string                      `json:"last_fail_reason,omitempty"`
	LastSyncAt     int64                       `json:"last_sync_at,omitempty"`
}

type legacyQuotaValue struct {
	Remaining     int                    `json:"remaining"`
	Total         int                    `json:"total"`
	Breakdown     []legacyQuotaBreakdown `json:"breakdown,omitempty"`
	WindowSeconds int                    `json:"window_seconds,omitempty"`
	ResetAt       *int64                 `json:"reset_at,omitempty"`
	SyncedAt      *int64                 `json:"synced_at,omitempty"`
	Source        string                 `json:"source,omitempty"`
}

type legacyQuotaBreakdown struct {
	ProductCode  int     `json:"product_code"`
	UsagePercent float64 `json:"usage_percent"`
}

type legacyTokenInput struct {
	Token  string `json:"token"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

type desiredLegacyToken struct {
	accountID uint64
	sourceKey string
	raw       string
	status    string
	note      string
}

func (h *Handler) registerTokens(admin *gin.RouterGroup) {
	admin.GET("/tokens", h.listLegacyTokens)
	admin.POST("/tokens", h.saveLegacyTokens)
	admin.POST("/tokens/refresh", h.refreshLegacyToken)
	admin.POST("/tokens/refresh/async", h.startLegacyQuotaBatch)
}

func (h *Handler) listLegacyTokens(c *gin.Context) {
	if h.accounts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Account service is not configured"})
		return
	}
	accounts, err := h.listAllWebAccounts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to list accounts"})
		return
	}
	result := map[string][]legacyTokenEntry{"ssoBasic": {}, "ssoSuper": {}, "ssoHeavy": {}}
	for _, view := range accounts {
		pool := legacyPool(view.Credential.WebTier)
		result[pool] = append(result[pool], newLegacyTokenEntry(view))
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) saveLegacyTokens(c *gin.Context) {
	if h.accounts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Account service is not configured"})
		return
	}
	var request map[string][]json.RawMessage
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid token payload"})
		return
	}
	desired := make([]desiredLegacyToken, 0)
	for _, values := range request {
		for _, rawValue := range values {
			var input legacyTokenInput
			if len(rawValue) > 0 && rawValue[0] == '"' {
				if json.Unmarshal(rawValue, &input.Token) != nil {
					c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid token entry"})
					return
				}
			} else if json.Unmarshal(rawValue, &input) != nil {
				c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid token entry"})
				return
			}
			input.Token = strings.TrimSpace(input.Token)
			if input.Token == "" {
				continue
			}
			item := desiredLegacyToken{status: input.Status, note: strings.TrimSpace(input.Note)}
			if id, ok := parseAccountHandle(input.Token); ok {
				item.accountID = id
			} else {
				item.raw = canonicalSSO(input.Token)
				if item.raw == "" {
					continue
				}
				item.sourceKey = sourceKeyForSSO(item.raw)
			}
			desired = append(desired, item)
		}
	}
	current, err := h.listAllWebAccounts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to read current accounts"})
		return
	}
	byID, bySource := indexWebAccounts(current)
	missing := make([]string, 0)
	for _, item := range desired {
		if item.accountID == 0 {
			if _, exists := bySource[item.sourceKey]; !exists {
				missing = append(missing, item.raw)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		if _, err := h.accounts.ImportWebCredentials(c.Request.Context(), []byte(strings.Join(missing, "\n"))); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
		current, err = h.listAllWebAccounts(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to reload imported accounts"})
			return
		}
		byID, bySource = indexWebAccounts(current)
	}
	keep := make(map[uint64]struct{}, len(desired))
	for _, item := range desired {
		view, exists := byID[item.accountID]
		if item.accountID == 0 {
			view, exists = bySource[item.sourceKey]
		}
		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "Imported account was not found"})
			return
		}
		keep[view.Credential.ID] = struct{}{}
		enabled := !strings.EqualFold(strings.TrimSpace(item.status), "disabled")
		update := accountapp.UpdateInput{Enabled: &enabled}
		if item.note != "" {
			update.Name = &item.note
		}
		if _, err := h.accounts.Update(c.Request.Context(), view.Credential.ID, update); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
			return
		}
	}
	for _, view := range current {
		if _, exists := keep[view.Credential.ID]; exists {
			continue
		}
		if err := h.accounts.Delete(c.Request.Context(), view.Credential.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *Handler) refreshLegacyToken(c *gin.Context) {
	if h.accounts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Account service is not configured"})
		return
	}
	var request struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(c.Request.Body).Decode(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid token"})
		return
	}
	id, ok := parseAccountHandle(request.Token)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Token refresh requires an account handle"})
		return
	}
	_, err := h.accounts.RefreshWebQuota(c.Request.Context(), id)
	c.JSON(http.StatusOK, gin.H{"status": "success", "results": gin.H{request.Token: err == nil}})
}

func (h *Handler) listAllWebAccounts(ctx context.Context) ([]accountapp.View, error) {
	values := make([]accountapp.View, 0)
	for page := 1; ; page++ {
		items, total, err := h.accounts.List(ctx, page, 100, "", accountapp.ListFilter{Provider: string(accountdomain.ProviderWeb)})
		if err != nil {
			return nil, err
		}
		values = append(values, items...)
		if len(values) >= int(total) || len(items) == 0 {
			return values, nil
		}
	}
}

func indexWebAccounts(values []accountapp.View) (map[uint64]accountapp.View, map[string]accountapp.View) {
	byID := make(map[uint64]accountapp.View, len(values))
	bySource := make(map[string]accountapp.View, len(values))
	for _, view := range values {
		byID[view.Credential.ID] = view
		if view.Credential.SourceKey != "" {
			bySource[view.Credential.SourceKey] = view
		}
	}
	return byID, bySource
}

func newLegacyTokenEntry(view accountapp.View) legacyTokenEntry {
	credential := view.Credential
	status := "active"
	if !credential.Enabled {
		status = "disabled"
	} else if credential.AuthStatus == accountdomain.AuthStatusReauthRequired {
		status = "invalid"
	} else if credential.CooldownUntil != nil && credential.CooldownUntil.After(time.Now().UTC()) {
		status = "cooling"
	}
	quota := make(map[string]legacyQuotaValue, len(view.QuotaWindows))
	latestSync := int64(0)
	for _, window := range view.QuotaWindows {
		mode := strings.TrimSpace(window.Mode)
		if mode == "" {
			continue
		}
		value := legacyQuotaValue{
			Remaining: window.Remaining, Total: window.Total, WindowSeconds: window.WindowSeconds,
			ResetAt: unixPointer(window.ResetAt), SyncedAt: unixPointer(window.SyncedAt), Source: string(window.Source),
		}
		for _, item := range window.Breakdown {
			value.Breakdown = append(value.Breakdown, legacyQuotaBreakdown{ProductCode: item.ProductCode, UsagePercent: item.UsagePercent})
		}
		quota[mode] = value
		if value.SyncedAt != nil && *value.SyncedAt > latestSync {
			latestSync = *value.SyncedAt
		}
	}
	return legacyTokenEntry{
		Token: accountHandle(credential.ID), Status: status, Quota: quota, Note: credential.Name,
		FailCount: credential.FailureCount, Tags: []string{}, CreatedAt: credential.CreatedAt.Unix(),
		LastUsedAt: unixValue(credential.LastUsedAt), LastFailReason: credential.LastError, LastSyncAt: latestSync,
	}
}

func legacyPool(tier accountdomain.WebTier) string {
	switch tier {
	case accountdomain.WebTierHeavy:
		return "ssoHeavy"
	case accountdomain.WebTierSuper:
		return "ssoSuper"
	default:
		return "ssoBasic"
	}
}

func accountHandle(id uint64) string { return "account:" + strconv.FormatUint(id, 10) }

func parseAccountHandle(raw string) (uint64, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "account:") {
		return 0, false
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(raw, "account:"), 10, 64)
	return id, err == nil && id > 0
}

func canonicalSSO(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "sso="))
}

func sourceKeyForSSO(raw string) string { return "sso:" + security.HashToken(canonicalSSO(raw)) }

func unixPointer(value *time.Time) *int64 {
	if value == nil || value.IsZero() {
		return nil
	}
	result := value.UTC().Unix()
	return &result
}

func unixValue(value *time.Time) int64 {
	if result := unixPointer(value); result != nil {
		return *result
	}
	return 0
}
