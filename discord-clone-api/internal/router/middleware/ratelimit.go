package middleware

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/logger"
)

// RateLimit enforces a per-IP request limit using keyPrefix:<ip> on domain.RateLimiter.
func RateLimit(limiter domain.RateLimiter, keyPrefix string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil || limit <= 0 {
			c.Next()
			return
		}

		key := keyPrefix + ":" + c.ClientIP()
		allowed, retryAfter, err := limiter.Allow(c.Request.Context(), key, limit, window)
		if err != nil {
			logger.FromContext(c.Request.Context()).Warn("rate limiter error, allowing request", "key", key, "err", err)
			c.Next()
			return
		}

		if !allowed {
			seconds := max(int(math.Ceil(retryAfter.Seconds())), 1)
			c.Header("Retry-After", strconv.Itoa(seconds))
			writeMiddlewareError(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, please try again later")
			return
		}

		c.Next()
	}
}
