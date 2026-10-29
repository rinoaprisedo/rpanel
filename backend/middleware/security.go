package middleware

import (
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORSMiddleware whitelists the configured frontend origin(s). frontendURLs
// is comma-separated (e.g. the React admin on :8801 and the Flutter web dev
// server on :8802 running side by side).
func CORSMiddleware(frontendURLs string) gin.HandlerFunc {
	var origins []string
	for _, o := range strings.Split(frontendURLs, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	cfg := cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-CSRF-Token", "Authorization"},
		AllowCredentials: true,
	}
	return cors.New(cfg)
}

const (
	defaultCSP = "default-src 'self'; script-src 'self'; style-src 'self'; " +
		"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; " +
		"frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
)

// SecurityHeaders adds the helmet-equivalent response headers.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", defaultCSP)
		c.Next()
	}
}

// RequestSizeLimit caps request body size (default 10MB).
func RequestSizeLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = MaxBytesReader(c, c.Request.Body, maxBytes)
		c.Next()
	}
}
