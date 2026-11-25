package middleware

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/truong/discord-clone-api/pkg/idgen"
	"github.com/truong/discord-clone-api/pkg/logger"
)

const headerRequestID = "X-Request-ID"

// RequestID ensures every request has a trace ID, exposes it via X-Request-ID,
// and attaches a request-scoped slog.Logger to the request context.
func RequestID(baseLogger *slog.Logger) gin.HandlerFunc {
	if baseLogger == nil {
		baseLogger = slog.Default()
	}

	return func(c *gin.Context) {
		reqID := strings.TrimSpace(c.GetHeader(headerRequestID))
		if !isValidRequestID(reqID) {
			reqID = idgen.MustUUIDv7().String()
		}

		c.Set(contextKeyRequestID, reqID)
		c.Header(headerRequestID, reqID)

		reqLogger := baseLogger.With("request_id", reqID)
		c.Request = c.Request.WithContext(logger.IntoContext(c.Request.Context(), reqLogger))

		c.Next()
	}
}

func isValidRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		b := id[i]
		if b < 0x21 || b > 0x7E {
			return false
		}
	}
	return true
}
