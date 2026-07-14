package legacy

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type ClientAuthenticator interface {
	Authenticate(context.Context, string) (clientkeydomain.Key, func(), error)
}

type Options struct {
	PublicEnabled bool
	AdminKey      string
	PublicKey     string
	ClientKey     string
	StorageType   string
}

type Handler struct {
	options    Options
	clientAuth ClientAuthenticator
}

func NewHandler(options Options, clientAuth ClientAuthenticator) *Handler {
	options.AdminKey = strings.TrimSpace(options.AdminKey)
	options.PublicKey = strings.TrimSpace(options.PublicKey)
	options.ClientKey = strings.TrimSpace(options.ClientKey)
	options.StorageType = strings.TrimSpace(options.StorageType)
	if options.StorageType == "" {
		options.StorageType = "sqlite"
	}
	return &Handler{options: options, clientAuth: clientAuth}
}

func (h *Handler) Register(router *gin.Engine, registerPublic, registerAdmin func(*gin.RouterGroup)) {
	public := router.Group("/v1/public")
	public.Use(h.publicAuth())
	public.GET("/verify", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	if registerPublic != nil {
		registerPublic(public)
	}

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
}

func (h *Handler) publicAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.options.PublicEnabled {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		raw := bearerToken(c.GetHeader("Authorization"))
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
