package legacy

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	settingsapp "github.com/chenyme/grok2api/backend/internal/application/settings"
	"github.com/gin-gonic/gin"
)

func (h *Handler) registerConfig(admin *gin.RouterGroup) {
	admin.GET("/config", h.getLegacyConfig)
	admin.POST("/config", h.updateLegacyConfig)
}

func (h *Handler) getLegacyConfig(c *gin.Context) {
	if h.settings == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Settings service is not configured"})
		return
	}
	c.JSON(http.StatusOK, legacyConfigSnapshot(h.settings.Get(), h.options.PublicEnabled))
}

func (h *Handler) updateLegacyConfig(c *gin.Context) {
	if h.settings == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Settings service is not configured"})
		return
	}
	var request map[string]any
	if json.NewDecoder(c.Request.Body).Decode(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid config payload"})
		return
	}
	current := h.settings.Get()
	updated, err := h.settings.Update(c.Request.Context(), current.Revision, applyLegacyConfigPatch(current.Config, request))
	if err != nil {
		switch {
		case errors.Is(err, settingsapp.ErrInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		case errors.Is(err, settingsapp.ErrConflict):
			c.JSON(http.StatusConflict, gin.H{"detail": "Settings changed; reload and retry"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to update settings"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "配置已更新", "config": legacyConfigSnapshot(updated, h.options.PublicEnabled)})
}

func legacyConfigSnapshot(snapshot settingsapp.Snapshot, publicEnabled bool) map[string]any {
	config := snapshot.Config
	return map[string]any{
		"app": map[string]any{
			"public_enabled": publicEnabled,
		},
		"proxy": map[string]any{
			"base_proxy_url": config.ProviderWeb.BaseURL,
			"statsig_id":     config.ProviderWeb.StatsigManualValue,
		},
		"retry": map[string]any{
			"max_retry":          config.Routing.MaxAttempts,
			"retry_backoff_base": config.Routing.CooldownBase,
			"retry_backoff_max":  config.Routing.CooldownMax,
		},
		"chat": map[string]any{
			"timeout": config.ProviderWeb.ChatTimeout,
		},
		"image": map[string]any{
			"timeout": config.ProviderWeb.ImageTimeout, "concurrent": config.ProviderWeb.MediaConcurrency,
			"nsfw": config.ProviderWeb.AllowNSFW,
		},
		"video": map[string]any{
			"timeout": config.ProviderWeb.VideoTimeout, "concurrent": config.ProviderWeb.MediaConcurrency,
		},
		"token": map[string]any{
			"import_concurrent": config.Batch.ImportConcurrency, "refresh_concurrent": config.Batch.RefreshConcurrency,
			"random_delay": config.Batch.RandomDelay,
		},
		"storage": map[string]any{
			"limit_mb": config.Media.MaxTotalBytes >> 20,
		},
		"cloakbrowser": map[string]any{
			"manual_statsig_id":            config.ProviderWeb.StatsigManualValue,
			"statsig_auto_refresh_enabled": false, "statsig_refresh_interval": 1800,
		},
	}
}

func applyLegacyConfigPatch(current settingsapp.EditableConfig, request map[string]any) settingsapp.EditableConfig {
	if section := legacySection(request, "proxy"); section != nil {
		if value, ok := legacyString(section, "base_proxy_url"); ok {
			current.ProviderWeb.BaseURL = value
		}
		if value, ok := legacyString(section, "statsig_id"); ok {
			current.ProviderWeb.StatsigManualValue = value
			current.ProviderWeb.StatsigManualConfigured = value != ""
			if value != "" {
				current.ProviderWeb.StatsigMode = "manual"
			}
		}
	}
	if section := legacySection(request, "image"); section != nil {
		if value, ok := legacyInt(section, "concurrent"); ok {
			current.ProviderWeb.MediaConcurrency = value
		}
		if value, ok := legacyBool(section, "nsfw"); ok {
			current.ProviderWeb.AllowNSFW = value
		}
		if value, ok := legacyString(section, "timeout"); ok {
			current.ProviderWeb.ImageTimeout = value
		}
	}
	if section := legacySection(request, "video"); section != nil {
		if value, ok := legacyString(section, "timeout"); ok {
			current.ProviderWeb.VideoTimeout = value
		}
	}
	if section := legacySection(request, "chat"); section != nil {
		if value, ok := legacyString(section, "timeout"); ok {
			current.ProviderWeb.ChatTimeout = value
		}
	}
	if section := legacySection(request, "retry"); section != nil {
		if value, ok := legacyInt(section, "max_retry"); ok {
			current.Routing.MaxAttempts = value
		}
		if value, ok := legacyString(section, "retry_backoff_base"); ok {
			current.Routing.CooldownBase = value
		}
		if value, ok := legacyString(section, "retry_backoff_max"); ok {
			current.Routing.CooldownMax = value
		}
	}
	if section := legacySection(request, "token"); section != nil {
		if value, ok := legacyInt(section, "import_concurrent"); ok {
			current.Batch.ImportConcurrency = value
		}
		if value, ok := legacyInt(section, "refresh_concurrent"); ok {
			current.Batch.RefreshConcurrency = value
		}
		if value, ok := legacyString(section, "random_delay"); ok {
			current.Batch.RandomDelay = value
		}
	}
	if section := legacySection(request, "storage"); section != nil {
		if value, ok := legacyInt(section, "limit_mb"); ok && value > 0 {
			current.Media.MaxTotalBytes = int64(value) << 20
		}
	}
	return current
}

func legacySection(request map[string]any, key string) map[string]any {
	value, _ := request[key].(map[string]any)
	return value
}

func legacyString(section map[string]any, key string) (string, bool) {
	value, exists := section[key]
	if !exists {
		return "", false
	}
	text, ok := value.(string)
	return strings.TrimSpace(text), ok
}

func legacyInt(section map[string]any, key string) (int, bool) {
	value, exists := section[key]
	if !exists {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func legacyBool(section map[string]any, key string) (bool, bool) {
	value, exists := section[key]
	if !exists {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return false, false
	}
}
