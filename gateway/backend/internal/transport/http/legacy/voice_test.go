package legacy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/gin-gonic/gin"
)

type fakeVoiceGateway struct {
	input gateway.VoiceTokenInput
	key   clientkeydomain.Key
}

func (f *fakeVoiceGateway) GenerateImage(context.Context, gateway.ImageGenerationInput) (*gateway.Result, error) {
	return &gateway.Result{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":[]}`))}, nil
}

func (f *fakeVoiceGateway) CreateVoiceToken(_ context.Context, key clientkeydomain.Key, input gateway.VoiceTokenInput) (gateway.VoiceTokenResult, error) {
	f.key, f.input = key, input
	return gateway.VoiceTokenResult{
		Token: "livekit-token", URL: "wss://livekit.grok.com", URLs: []string{"wss://livekit.grok.com"},
		ParticipantName: "guest", RoomName: "room", ICEServers: []map[string]any{{"urls": []string{"stun:stun.example.com"}}},
	}, nil
}

func TestVoiceTokenUsesGoGatewayAndBuildsSameOriginSignalProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeClientAuthenticator{wantRaw: "g2-direct-key"}
	voice := &fakeVoiceGateway{}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, voice)
	router := gin.New()
	handler.Register(router, nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/v1/public/voice/token?voice=eve&personality=therapist&speed=1.4", nil)
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "grok.example.com")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{`"token":"livekit-token"`, `"url":"wss://livekit.grok.com"`, `"participant_name":"guest"`, `"room_name":"room"`, `"signal_proxy_url":"wss://grok.example.com/v1/public/voice/signal"`, `"ice_servers"`} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("body missing %s: %s", expected, recorder.Body.String())
		}
	}
	if voice.key.ID != 7 || voice.input.Voice != "eve" || voice.input.Personality != "therapist" || voice.input.Speed != 1.4 {
		t.Fatalf("key=%#v input=%#v", voice.key, voice.input)
	}
}

func TestVoiceTokenValidatesSpeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(Options{PublicEnabled: true}, &fakeClientAuthenticator{wantRaw: "g2-direct-key"}, &fakeVoiceGateway{})
	router := gin.New()
	handler.Register(router, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/public/voice/token?speed=9", nil)
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
