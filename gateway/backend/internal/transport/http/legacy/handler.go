package legacy

import (
	"context"
	"crypto/subtle"
	"net/http"
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
	StorageType         string
	AllowNSFW           bool
	VideoPollInterval   time.Duration
	Accounts            LegacyAccountService
	Settings            *settingsapp.Service
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
	videoCache          LegacyVideoCache
	videoReferenceStore VideoReferenceStore
}

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
		videoCache: options.VideoCache, videoReferenceStore: options.VideoReferenceStore,
	}
}

func (h *Handler) Register(router *gin.Engine, registerPublic, registerAdmin func(*gin.RouterGroup)) {
	router.GET("/v1/public/imagine/config", h.imagineConfig)
	public := router.Group("/v1/public")
	public.Use(h.publicAuth())
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
		if raw == "" {
			raw = strings.TrimSpace(c.Query("public_key"))
		}
		if h.options.PublicKey != "" && !constantTimeEqual(raw, h.options.PublicKey) {
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
