// Package middleware contains Gin middleware: logging, CORS, (later) auth.
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logging returns a Gin middleware that emits one structured zap line per
// request. The shape is stable across handlers — pkg/api handlers should NOT
// re-log request metadata, only business events.
func Logging(logger *zap.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		fullPath := path
		if raw != "" {
			fullPath = path + "?" + raw
		}

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", fullPath),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("clientIp", c.ClientIP()),
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
			logger.Error("request", fields...)
			return
		}
		logger.Info("request", fields...)
	}
}
