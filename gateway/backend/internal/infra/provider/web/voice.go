package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domainegress "github.com/chenyme/grok2api/backend/internal/domain/egress"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

const defaultLiveKitURL = "wss://livekit.grok.com"

func (a *Adapter) CreateVoiceToken(ctx context.Context, request provider.VoiceTokenRequest) (provider.VoiceTokenResult, error) {
	cfg := a.config()
	token, err := a.cipher.Decrypt(request.Credential.EncryptedAccessToken)
	if err != nil {
		return provider.VoiceTokenResult{}, err
	}
	lease, err := a.egress.AcquireCredential(ctx, domainegress.ScopeWeb, request.Credential)
	if err != nil {
		return provider.VoiceTokenResult{}, err
	}
	defer lease.Release()
	sessionPayload, _ := json.Marshal(map[string]any{
		"voice": request.Voice, "personality": request.Personality, "playback_speed": request.Speed,
		"enable_vision": false, "turn_detection": map[string]any{"type": "server_vad"},
	})
	payload := map[string]any{
		"sessionPayload": string(sessionPayload), "requestAgentDispatch": false, "livekitUrl": defaultLiveKitURL,
		"params": map[string]string{"enable_markdown_transcript": "true"},
	}
	response, err := a.postJSONWithReferer(ctx, cfg, lease, token, cfg.BaseURL+"/rest/livekit/tokens", payload, time.Duration(cfg.ChatTimeoutSeconds)*time.Second, cfg.BaseURL+"/")
	if err != nil {
		return provider.VoiceTokenResult{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return provider.VoiceTokenResult{}, err
	}
	if response.StatusCode == http.StatusUnauthorized {
		return provider.VoiceTokenResult{}, provider.ErrUnauthorized
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return provider.VoiceTokenResult{}, fmt.Errorf("LiveKit Token 上游返回 %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return provider.VoiceTokenResult{}, fmt.Errorf("LiveKit Token 返回无效 JSON")
	}
	result := provider.VoiceTokenResult{
		Token:           firstNestedString(raw, []string{"token"}, []string{"access_token"}),
		URL:             normalizeLiveKitURL(firstNestedString(raw, []string{"url"}, []string{"livekitUrl"}, []string{"livekit_url"}, []string{"connection", "url"})),
		ParticipantName: firstNestedString(raw, []string{"participant_name"}, []string{"participantName"}, []string{"identity"}),
		RoomName:        firstNestedString(raw, []string{"room_name"}, []string{"roomName"}, []string{"room"}),
		ICEServers:      normalizeICEServers(raw),
	}
	result.URLs = normalizeLiveKitURLs(raw, result.URL)
	if result.URL == "" {
		result.URL = defaultLiveKitURL
	}
	if len(result.URLs) == 0 {
		result.URLs = []string{result.URL}
	}
	if result.Token == "" {
		return provider.VoiceTokenResult{}, fmt.Errorf("LiveKit Token 上游未返回 token")
	}
	return result, nil
}

func firstNestedString(root map[string]any, paths ...[]string) string {
	for _, path := range paths {
		var value any = root
		for _, key := range path {
			node, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = node[key]
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func normalizeLiveKitURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "wss://" + value
	}
	if !strings.HasPrefix(value, "ws://") && !strings.HasPrefix(value, "wss://") {
		return ""
	}
	return strings.TrimRight(value, "/")
}

func normalizeLiveKitURLs(root map[string]any, primary string) []string {
	values := []string{}
	add := func(value string) {
		value = normalizeLiveKitURL(value)
		if value == "" {
			return
		}
		for _, existing := range values {
			if existing == value {
				return
			}
		}
		values = append(values, value)
	}
	add(primary)
	for _, key := range []string{"urls", "livekitUrls", "livekit_urls"} {
		if list, ok := root[key].([]any); ok {
			for _, item := range list {
				if text, ok := item.(string); ok {
					add(text)
				}
			}
		}
	}
	add(defaultLiveKitURL)
	return values
}

func normalizeICEServers(root map[string]any) []map[string]any {
	var raw any = root["iceServers"]
	if raw == nil {
		raw = root["ice_servers"]
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(list))
	for _, item := range list {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		urls := node["urls"]
		if urls == nil {
			urls = node["url"]
		}
		if urls == nil {
			continue
		}
		entry := map[string]any{"urls": urls}
		if value := node["username"]; value != nil {
			entry["username"] = value
		}
		if value := node["credential"]; value != nil {
			entry["credential"] = value
		}
		result = append(result, entry)
	}
	return result
}

var _ provider.VoiceAdapter = (*Adapter)(nil)
