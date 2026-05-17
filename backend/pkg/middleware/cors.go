package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORSConfig customizes which origins / methods / headers are allowed. Phase-1
// dev defaults to "*" everywhere; production deployments should narrow this.
type CORSConfig struct {
	AllowOrigins []string
	AllowMethods []string
	AllowHeaders []string
}

// DefaultCORSConfig is the permissive dev default.
//
//nolint:gochecknoglobals // intentional default value
var DefaultCORSConfig = CORSConfig{
	AllowOrigins: []string{"*"},
	AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
	AllowHeaders: []string{"Origin", "Content-Type", "Authorization", "Accept"},
}

// CORS returns a minimal CORS middleware. We deliberately avoid pulling
// gin-contrib/cors for the scaffold — it adds a heavy dep tree and our needs
// are small. Replace later if we need credentialed CORS, preflight caching, etc.
func CORS(cfg CORSConfig) gin.HandlerFunc {
	allowOrigin := "*"
	if len(cfg.AllowOrigins) > 0 {
		allowOrigin = joinComma(cfg.AllowOrigins)
	}
	allowMethods := joinComma(cfg.AllowMethods)
	allowHeaders := joinComma(cfg.AllowHeaders)

	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", allowMethods)
		c.Writer.Header().Set("Access-Control-Allow-Headers", allowHeaders)

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func joinComma(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
