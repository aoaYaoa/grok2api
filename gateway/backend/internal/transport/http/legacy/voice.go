package legacy

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type VoiceGateway interface {
	CreateVoiceToken(context.Context, clientkeydomain.Key, gateway.VoiceTokenInput) (gateway.VoiceTokenResult, error)
}

func (h *Handler) registerVoice(public *gin.RouterGroup) {
	public.GET("/voice/token", h.voiceToken)
	public.GET("/voice/signal", h.voiceSignal)
	public.GET("/voice/signal/*tail", h.voiceSignal)
}

func (h *Handler) voiceToken(c *gin.Context) {
	if h.voiceGateway == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Voice gateway is not configured"})
		return
	}
	clientValue, ok := c.Get(middleware.ClientKey)
	clientKey, valid := clientValue.(clientkeydomain.Key)
	if !ok || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
		return
	}
	voice := strings.TrimSpace(c.DefaultQuery("voice", "ara"))
	personality := strings.TrimSpace(c.DefaultQuery("personality", "assistant"))
	speed, err := strconv.ParseFloat(c.DefaultQuery("speed", "1"), 64)
	if err != nil || speed < 0.5 || speed > 2 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "speed must be between 0.5 and 2.0"})
		return
	}
	result, err := h.voiceGateway.CreateVoiceToken(c.Request.Context(), clientKey, gateway.VoiceTokenInput{Voice: voice, Personality: personality, Speed: speed})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": result.Token, "url": result.URL, "urls": result.URLs, "participant_name": result.ParticipantName, "room_name": result.RoomName, "ice_servers": result.ICEServers, "signal_proxy_url": voiceSignalProxyURL(c.Request)})
}

func voiceSignalProxyURL(request *http.Request) string {
	host := strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-Host"), ",")[0])
	if host == "" {
		host = request.Host
	}
	if host == "" {
		return ""
	}
	proto := strings.ToLower(strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-Proto"), ",")[0]))
	if proto == "" {
		if request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	scheme := "ws"
	if proto == "https" {
		scheme = "wss"
	}
	return scheme + "://" + host + "/v1/public/voice/signal"
}

var voiceUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, HandshakeTimeout: 15 * time.Second}

func (h *Handler) voiceSignal(c *gin.Context) {
	client, err := voiceUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer client.Close()
	tail := strings.Trim(strings.TrimSpace(c.Param("tail")), "/")
	target := "wss://livekit.grok.com"
	if tail != "" {
		target += "/" + tail
	}
	query := c.Request.URL.Query()
	query.Del("public_key")
	query.Del("upstream")
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	header := http.Header{"Origin": []string{"https://grok.com"}, "User-Agent": []string{c.Request.UserAgent()}}
	dialer := websocket.Dialer{HandshakeTimeout: 20 * time.Second, Proxy: http.ProxyFromEnvironment}
	upstream, response, err := dialer.DialContext(c.Request.Context(), target, header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream unavailable"), time.Now().Add(time.Second))
		return
	}
	defer upstream.Close()
	done := make(chan struct{}, 2)
	relay := func(destination, source *websocket.Conn) {
		defer func() { done <- struct{}{} }()
		for {
			messageType, data, readErr := source.ReadMessage()
			if readErr != nil {
				return
			}
			if writeErr := destination.WriteMessage(messageType, data); writeErr != nil {
				return
			}
		}
	}
	go relay(upstream, client)
	go relay(client, upstream)
	select {
	case <-done:
	case <-c.Request.Context().Done():
	}
}

func validVoiceSignalTarget(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "wss" && strings.EqualFold(parsed.Hostname(), "livekit.grok.com")
}
