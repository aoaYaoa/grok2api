package httpserver

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

var publicFrontendRoutes = []string{"/login", "/chat", "/imagine", "/imagine-workbench", "/video", "/nsfw", "/voice"}

func registerPublicFrontend(router *gin.Engine, staticPath string, enabled bool) {
	router.GET("/", func(c *gin.Context) {
		if enabled {
			c.Redirect(http.StatusTemporaryRedirect, "/login")
		} else {
			c.Redirect(http.StatusTemporaryRedirect, "/gateway/login")
		}
	})
	root, indexPath, ok := frontendRoot(staticPath)
	if !ok || !enabled {
		return
	}
	serveIndex := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		http.ServeFile(c.Writer, c.Request, indexPath)
	}
	for _, route := range publicFrontendRoutes {
		router.GET(route, serveIndex)
		router.HEAD(route, serveIndex)
	}
	asset := func(c *gin.Context) {
		requestPath := strings.TrimPrefix(c.Param("filepath"), "/")
		filePath, exists := frontendFile(root, path.Join("assets", requestPath))
		if !exists {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(c.Writer, c.Request, filepath.Join(root, filePath))
	}
	router.GET("/assets/*filepath", asset)
	router.HEAD("/assets/*filepath", asset)
	router.GET("/sw.js", func(c *gin.Context) {
		filePath, exists := frontendFile(root, "/sw.js")
		if !exists {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "no-store, max-age=0")
		c.Header("Service-Worker-Allowed", "/")
		http.ServeFile(c.Writer, c.Request, filepath.Join(root, filePath))
	})
	for _, assetPath := range []string{"/grok2api.png", "/manifest.webmanifest", "/favicon.ico"} {
		assetPath := assetPath
		router.GET(assetPath, func(c *gin.Context) {
			filePath, exists := frontendFile(root, assetPath)
			if !exists {
				c.Status(http.StatusNotFound)
				return
			}
			c.Header("Cache-Control", "no-cache")
			http.ServeFile(c.Writer, c.Request, filepath.Join(root, filePath))
		})
	}
}

func frontendBuildPath(root, build string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	candidate := filepath.Join(root, build)
	if _, _, ok := frontendRoot(candidate); ok {
		return candidate
	}
	if build == "admin" {
		return root
	}
	return candidate
}
