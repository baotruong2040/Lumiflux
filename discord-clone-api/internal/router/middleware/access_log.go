package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/truong/discord-clone-api/pkg/logger"
)

// AccessLog logs HTTP request metadata and duration using the request-scoped slog.Logger.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		logger.FromContext(c.Request.Context()).Info("http request",
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}
