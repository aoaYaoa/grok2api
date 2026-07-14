package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const legacyAssetPlaceholder = "__ASSET_VERSION__"

var legacyPageRoutes = map[string]string{
	"/login":             "public/pages/login.html",
	"/chat":              "public/pages/chat.html",
	"/imagine":           "public/pages/imagine.html",
	"/imagine-workbench": "public/pages/imagine_workbench.html",
	"/video":             "public/pages/video.html",
	"/nsfw":              "public/pages/nsfw.html",
	"/voice":             "public/pages/voice.html",
	"/admin/login":       "admin/pages/login.html",
	"/admin/token":       "admin/pages/token.html",
	"/admin/config":      "admin/pages/config.html",
	"/admin/cache":       "admin/pages/cache.html",
}

func registerLegacyPages(router *gin.Engine, staticPath, assetVersion string, publicEnabled bool) {
	root, ok := legacyRoot(staticPath)
	if !ok {
		return
	}
	assetVersion = strings.TrimSpace(assetVersion)
	if assetVersion == "" {
		assetVersion = "dev"
	}

	router.GET("/", func(c *gin.Context) {
		if publicEnabled {
			c.Redirect(http.StatusTemporaryRedirect, "/login")
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, "/admin/login")
	})
	router.GET("/admin", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/admin/login")
	})

	for route, relativePath := range legacyPageRoutes {
		route := route
		relativePath := relativePath
		router.GET(route, func(c *gin.Context) {
			if strings.HasPrefix(route, "/") && !strings.HasPrefix(route, "/admin/") && !publicEnabled {
				c.Status(http.StatusNotFound)
				return
			}
			serveLegacyHTML(c, root, relativePath, assetVersion)
		})
	}

	staticHandler := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		serveLegacyFile(c, root, c.Param("filepath"), "")
	}
	router.GET("/static/*filepath", staticHandler)
	router.HEAD("/static/*filepath", staticHandler)

	router.GET("/manifest.webmanifest", func(c *gin.Context) {
		if !publicEnabled {
			c.Status(http.StatusNotFound)
			return
		}
		serveLegacyFile(c, root, "/public/manifest.webmanifest", "application/manifest+json")
	})
	router.GET("/sw.js", func(c *gin.Context) {
		if !publicEnabled {
			c.Status(http.StatusNotFound)
			return
		}
		serveLegacyFile(c, root, "/public/sw.js", "application/javascript")
	})
	router.GET("/favicon.ico", func(c *gin.Context) {
		serveLegacyFile(c, root, "/common/img/favicon/favicon.ico", "image/x-icon")
	})
}

func legacyRoot(staticPath string) (string, bool) {
	staticPath = strings.TrimSpace(staticPath)
	if staticPath == "" {
		return "", false
	}
	root, err := filepath.Abs(staticPath)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return filepath.Clean(root), true
}

func serveLegacyHTML(c *gin.Context, root, relativePath, assetVersion string) {
	filePath, ok := legacyFile(root, relativePath)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store, max-age=0")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(strings.ReplaceAll(string(content), legacyAssetPlaceholder, assetVersion)))
}

func serveLegacyFile(c *gin.Context, root, relativePath, contentType string) {
	filePath, ok := legacyFile(root, relativePath)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}
	http.ServeFile(c.Writer, c.Request, filePath)
}

func legacyFile(root, relativePath string) (string, bool) {
	cleanPath := strings.TrimPrefix(filepath.Clean("/"+relativePath), string(filepath.Separator))
	fullPath := filepath.Join(root, cleanPath)
	relative, err := filepath.Rel(root, fullPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(fullPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return fullPath, true
}
