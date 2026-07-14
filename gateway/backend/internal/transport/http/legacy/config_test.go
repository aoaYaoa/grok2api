package legacy

import (
	"testing"

	settingsapp "github.com/chenyme/grok2api/backend/internal/application/settings"
)

func TestLegacyConfigSnapshotExposesOnlyGoBackedSections(t *testing.T) {
	snapshot := settingsapp.Snapshot{Revision: 7, Config: settingsapp.EditableConfig{
		ProviderWeb: settingsapp.ProviderWebConfig{BaseURL: "https://grok.com", StatsigMode: "manual", StatsigManualValue: "statsig", StatsigManualConfigured: true, ChatTimeout: "15m", ImageTimeout: "15m", VideoTimeout: "2h", MediaConcurrency: 4, AllowNSFW: true},
		Batch:       settingsapp.BatchConfig{ImportConcurrency: 3, RefreshConcurrency: 5, RandomDelay: "250ms"},
		Media:       settingsapp.MediaConfig{MaxImageBytes: 32 << 20, MaxTotalBytes: 10 << 30, CleanupThresholdPercent: 80, CleanupInterval: "1h"},
		Routing:     settingsapp.RoutingConfig{CooldownBase: "30s", CooldownMax: "5m", MaxAttempts: 3},
	}}
	result := legacyConfigSnapshot(snapshot, true)
	if result["app"].(map[string]any)["public_enabled"] != true {
		t.Fatalf("app=%v", result["app"])
	}
	if result["image"].(map[string]any)["nsfw"] != true || result["image"].(map[string]any)["concurrent"] != 4 {
		t.Fatalf("image=%v", result["image"])
	}
	if result["proxy"].(map[string]any)["statsig_id"] != "statsig" {
		t.Fatalf("proxy=%v", result["proxy"])
	}
}

func TestApplyLegacyConfigPatchUpdatesSupportedGoFields(t *testing.T) {
	current := settingsapp.EditableConfig{
		ProviderWeb: settingsapp.ProviderWebConfig{BaseURL: "https://grok.com", StatsigMode: "url", MediaConcurrency: 4},
		Batch:       settingsapp.BatchConfig{ImportConcurrency: 2, RefreshConcurrency: 2},
		Media:       settingsapp.MediaConfig{MaxTotalBytes: 10 << 30},
		Routing:     settingsapp.RoutingConfig{MaxAttempts: 3},
	}
	updated := applyLegacyConfigPatch(current, map[string]any{
		"proxy":   map[string]any{"base_proxy_url": "https://example.com", "statsig_id": "manual-value"},
		"image":   map[string]any{"concurrent": float64(6), "nsfw": true},
		"retry":   map[string]any{"max_retry": float64(5)},
		"token":   map[string]any{"refresh_concurrent": float64(7)},
		"storage": map[string]any{"limit_mb": float64(2048)},
	})
	if updated.ProviderWeb.BaseURL != "https://example.com" || updated.ProviderWeb.StatsigMode != "manual" || updated.ProviderWeb.StatsigManualValue != "manual-value" || !updated.ProviderWeb.AllowNSFW || updated.ProviderWeb.MediaConcurrency != 6 {
		t.Fatalf("provider=%+v", updated.ProviderWeb)
	}
	if updated.Routing.MaxAttempts != 5 || updated.Batch.RefreshConcurrency != 7 || updated.Media.MaxTotalBytes != 2048<<20 {
		t.Fatalf("updated=%+v", updated)
	}
}
