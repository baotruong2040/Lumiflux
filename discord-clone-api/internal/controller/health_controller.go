package controller

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/logger"
)

// HealthController handles liveness (/healthz) and readiness (/readyz) checks.
type HealthController struct {
	pg    domain.HealthChecker
	redis domain.HealthChecker
	log   *slog.Logger
}

// NewHealthController constructs a HealthController for PostgreSQL and Redis probes.
func NewHealthController(pg, redis domain.HealthChecker, log *slog.Logger) *HealthController {
	return &HealthController{
		pg:    pg,
		redis: redis,
		log:   log,
	}
}

// Healthz handles GET /healthz and reports that the HTTP server process is alive.
func (h *HealthController) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readyz handles GET /readyz and verifies connectivity to PostgreSQL and Redis.
func (h *HealthController) Readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{
		"postgres": "ok",
		"redis":    "ok",
	}
	ready := true

	reqLog := logger.FromContext(c.Request.Context())
	if reqLog == slog.Default() && h.log != nil {
		reqLog = h.log
	}

	if h.pg == nil {
		checks["postgres"] = "unavailable"
		ready = false
	} else if err := h.pg.Ping(ctx); err != nil {
		checks["postgres"] = "unavailable"
		ready = false
		reqLog.Warn("postgres readiness check failed", "err", err)
	}

	if h.redis == nil {
		checks["redis"] = "unavailable"
		ready = false
	} else if err := h.redis.Ping(ctx); err != nil {
		checks["redis"] = "unavailable"
		ready = false
		reqLog.Warn("redis readiness check failed", "err", err)
	}

	if !ready {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"checks": checks,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": checks,
	})
}
