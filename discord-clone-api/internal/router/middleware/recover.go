package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/truong/discord-clone-api/pkg/logger"
)

// Recover catches panics in HTTP handlers, logs the stack trace, and responds with HTTP 500 JSON.
func Recover(baseLogger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				reqLog := logger.FromContext(c.Request.Context())
				if reqLog == slog.Default() && baseLogger != nil {
					reqLog = baseLogger
				}
				reqLog.Error("panic recovered",
					"panic", rec,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"stack", string(debug.Stack()),
				)
				if !c.Writer.Written() {
					writeMiddlewareError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "an internal server error occurred")
				} else {
					c.Abort()
				}
			}
		}()

		c.Next()
	}
}
