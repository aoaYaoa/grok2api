package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

var legacyAdminRedirects = map[string]string{
	"/admin":        "/gateway/dashboard",
	"/admin/login":  "/gateway/login",
	"/admin/token":  "/gateway/accounts",
	"/admin/config": "/gateway/settings",
	"/admin/cache":  "/gateway/cache",
}

func registerLegacyPages(router *gin.Engine, _, _ string, _ bool) {
	redirectAdmin := func(c *gin.Context) {
		destination := legacyAdminRedirects[c.Request.URL.Path]
		if destination == "" {
			destination = "/gateway/dashboard"
		}
		c.Redirect(http.StatusTemporaryRedirect, destination)
	}
	router.GET("/admin", redirectAdmin)
	router.GET("/admin/*path", redirectAdmin)
}
