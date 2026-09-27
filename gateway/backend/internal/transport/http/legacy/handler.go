package legacy

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	settingsapp "github.com/chenyme/grok2api/backend/internal/application/settings"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type ClientAuthenticator interface {
	Authenticate(context.Context, string) (clientkeydomain.Key, func(), error)
}

type Options struct {
	PublicEnabled       bool
	AdminKey            string
	PublicKey           string
	ClientKey           string
	PublicAuthUsername  string
	PublicAuthPassword  string
	PublicAuthSecret    string
	SecureCookies       bool
	StorageType         string
	AllowNSFW           bool
	VideoPollInterval   time.Duration
	Accounts            LegacyAccountService
	Settings            *settingsapp.Service
	ImageCache          LegacyImageCache
	VideoCache          LegacyVideoCache
	VideoReferenceStore VideoReferenceStore
}

type Handler struct {
	options             Options
	clientAuth          ClientAuthenticator
	imageGenerator      ImageGenerator
	imageMu             sync.Mutex
	imageTasks          map[string]*imageTask
	videoGateway        VideoGateway
	voiceGateway        VoiceGateway
	videoMu             sync.Mutex
	videoTasks          map[string]*videoTask
	accounts            LegacyAccountService
	batchMu             sync.RWMutex
	batchTasks          map[string]*legacyBatchTask
	promptGateway       PromptGateway
	promptMu            sync.Mutex
	promptTasks         map[string]*promptTask
	promptTaskTTL       time.Duration
	settings            *settingsapp.Service
	imageCache          LegacyImageCache
	videoCache          LegacyVideoCache
	videoReferenceStore VideoReferenceStore
	sessionMu           sync.Mutex
	revokedSessions     map[string]time.Time
}

const (
	publicSessionCookieName = "grok2api_public_session"
	publicSessionTTL        = 7 * 24 * time.Hour
)

type ImageGenerator interface {
	GenerateImage(context.Context, gateway.ImageGenerationInput) (*gateway.Result, error)
}

type ImageEditor interface {
	EditImage(context.Context, gateway.ImageEditInput) (*gateway.Result, error)
}

type PromptGateway interface {
	CreateChatCompletion(context.Context, gateway.Input) (*gateway.Result, error)
}

func NewHandler(options Options, clientAuth ClientAuthenticator, imageGenerator ...ImageGenerator) *Handler {
	options.AdminKey = strings.TrimSpace(options.AdminKey)
	options.PublicKey = strings.TrimSpace(options.PublicKey)
	options.ClientKey = strings.TrimSpace(options.ClientKey)
	options.StorageType = strings.TrimSpace(options.StorageType)
	if options.StorageType == "" {
		options.StorageType = "sqlite"
	}
	if options.VideoPollInterval <= 0 {
		options.VideoPollInterval = time.Second
	}
	var generator ImageGenerator
	if len(imageGenerator) > 0 {
		generator = imageGenerator[0]
	}
	var videoGateway VideoGateway
	if candidate, ok := generator.(VideoGateway); ok {
		videoGateway = candidate
	}
	var promptGateway PromptGateway
	if candidate, ok := generator.(PromptGateway); ok {
		promptGateway = candidate
	}
	var voiceGateway VoiceGateway
	if candidate, ok := generator.(VoiceGateway); ok {
		voiceGateway = candidate
	}
	return &Handler{
		options: options, clientAuth: clientAuth, imageGenerator: generator, imageTasks: make(map[string]*imageTask),
		videoGateway: videoGateway, voiceGateway: voiceGateway, videoTasks: make(map[string]*videoTask), accounts: options.Accounts,
		batchTasks: make(map[string]*legacyBatchTask), promptGateway: promptGateway,
		promptTasks: make(map[string]*promptTask), promptTaskTTL: 5 * time.Minute, settings: options.Settings,
		imageCache: options.ImageCache, videoCache: options.VideoCache, videoReferenceStore: options.VideoReferenceStore,
		revokedSessions: make(map[string]time.Time),
	}
}

func (h *Handler) Register(router *gin.Engine, registerPublic, registerAdmin func(*gin.RouterGroup)) {
	router.POST("/v1/public/auth/login", h.publicLogin)
	router.POST("/v1/public/auth/logout", h.publicLogout)
	public := router.Group("/v1/public")
	public.Use(h.publicAuth())
	public.GET("/imagine/config", h.imagineConfig)
	public.GET("/verify", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	if registerPublic != nil {
		registerPublic(public)
	}
	h.registerImagine(public)
	h.registerVideo(public)
	h.registerPrompt(public)
	h.registerVoice(public)

	admin := router.Group("/v1/admin")
	admin.Use(h.adminAuth())
	admin.GET("/verify", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	admin.GET("/storage", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"type": h.options.StorageType})
	})
	if registerAdmin != nil {
		registerAdmin(admin)
	}
	h.registerTokens(admin)
	h.registerBatchTasks(admin)
	h.registerConfig(admin)
}

var _ LegacyAccountService = (*accountapp.Service)(nil)

func (h *Handler) publicAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.options.PublicEnabled {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		raw := bearerToken(c.GetHeader("Authorization"))
		sessionAuthenticated := h.validPublicSession(c)
		if sessionAuthenticated {
			raw = h.options.ClientKey
		}
		if raw == "" {
			raw = strings.TrimSpace(c.Query("public_key"))
		}
		if h.publicPasswordAuthConfigured() && !sessionAuthenticated &&
			(h.options.PublicKey == "" || !constantTimeEqual(raw, h.options.PublicKey)) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "请先登录公共工作台"})
			return
		}
		if h.options.PublicKey != "" && !sessionAuthenticated && !constantTimeEqual(raw, h.options.PublicKey) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
			return
		}
		clientRaw := h.options.ClientKey
		if clientRaw == "" {
			clientRaw = raw
		}
		if clientRaw == "" || h.clientAuth == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Client key is not configured"})
			return
		}
		value, release, err := h.clientAuth.Authenticate(c.Request.Context(), clientRaw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Invalid public key"})
			return
		}
		if release != nil {
			defer release()
		}
		c.Set(middleware.ClientKey, value)
		c.Next()
	}
}

func (h *Handler) publicPasswordAuthConfigured() bool {
	return strings.TrimSpace(h.options.PublicAuthUsername) != "" &&
		strings.TrimSpace(h.options.PublicAuthPassword) != "" &&
		strings.TrimSpace(h.options.PublicAuthSecret) != ""
}

type publicLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) publicLogin(c *gin.Context) {
	if !h.options.PublicEnabled {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var input publicLoginRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"detail": "用户名或密码格式无效"})
		return
	}
	if h.options.PublicAuthUsername == "" || h.options.PublicAuthPassword == "" ||
		!constantTimeEqual(strings.TrimSpace(input.Username), h.options.PublicAuthUsername) ||
		!constantTimeEqual(input.Password, h.options.PublicAuthPassword) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "用户名或密码错误"})
		return
	}
	if strings.TrimSpace(h.options.PublicAuthSecret) == "" {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"detail": "公共登录尚未配置"})
		return
	}
	token, err := h.newPublicSession()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"detail": "无法创建登录会话"})
		return
	}
	h.setPublicSessionCookie(c, token, publicSessionTTL)
	c.JSON(http.StatusOK, gin.H{"authenticated": true})
}

func (h *Handler) publicLogout(c *gin.Context) {
	if cookie, err := c.Cookie(publicSessionCookieName); err == nil {
		if expiresAt, ok := h.publicSessionExpiry(cookie); ok {
			h.sessionMu.Lock()
			h.revokedSessions[cookie] = expiresAt
			h.sessionMu.Unlock()
		}
	}
	h.setPublicSessionCookie(c, "", -time.Hour)
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}

func (h *Handler) newPublicSession() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%d.%s", time.Now().Add(publicSessionTTL).Unix(), base64.RawURLEncoding.EncodeToString(value))
	mac := hmac.New(sha256.New, []byte(h.options.PublicAuthSecret))
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (h *Handler) validPublicSession(c *gin.Context) bool {
	cookie, err := c.Cookie(publicSessionCookieName)
	if err != nil || cookie == "" {
		return false
	}
	expiresAt, ok := h.publicSessionExpiry(cookie)
	if !ok || !expiresAt.After(time.Now()) {
		return false
	}
	h.sessionMu.Lock()
	for token, revokedUntil := range h.revokedSessions {
		if !revokedUntil.After(time.Now()) {
			delete(h.revokedSessions, token)
		}
	}
	_, revoked := h.revokedSessions[cookie]
	h.sessionMu.Unlock()
	return !revoked
}

func (h *Handler) publicSessionExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || strings.TrimSpace(h.options.PublicAuthSecret) == "" {
		return time.Time{}, false
	}
	mac := hmac.New(sha256.New, []byte(h.options.PublicAuthSecret))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return time.Time{}, false
	}
	unix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(unix, 0), true
}

func (h *Handler) setPublicSessionCookie(c *gin.Context, value string, maxAge time.Duration) {
	seconds := int(maxAge.Seconds())
	if seconds < 0 {
		seconds = -1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: publicSessionCookieName, Value: value, Path: "/", MaxAge: seconds,
		HttpOnly: true, Secure: h.options.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearerToken(c.GetHeader("Authorization"))
		if h.options.AdminKey == "" || !constantTimeEqual(raw, h.options.AdminKey) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Invalid admin key"})
			return
		}
		c.Next()
	}
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func constantTimeEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
