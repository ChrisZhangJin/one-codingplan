package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:web_dist
var webFS embed.FS

// spaHandler serves the embedded SPA files. For any path not matching a
// static file, it falls back to index.html (SPA client-side routing).
func spaHandler() gin.HandlerFunc {
	sub, err := fs.Sub(webFS, "web_dist")
	if err != nil {
		// web_dist is empty — portal not built, skip SPA serving
		return func(c *gin.Context) { c.Next() }
	}

	fileServer := http.FileServer(http.FS(sub))

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/v1") || path == "/health" {
			c.Next()
			return
		}

		f, err := sub.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}

		// Fall back to index.html for SPA routing
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
