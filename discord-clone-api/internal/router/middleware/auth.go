package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/truong/discord-clone-api/internal/domain"
)

// Auth validates the Bearer JWT access token and stores the user ID in context.
func Auth(tokens domain.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			writeMiddlewareError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing authorization header")
			return
		}

		scheme, token, ok := strings.Cut(authHeader, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			writeMiddlewareError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid authorization header format")
			return
		}

		userID, err := tokens.ParseAccessToken(strings.TrimSpace(token))
		if err != nil {
			writeMiddlewareError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired access token")
			return
		}

		SetUserID(c, userID)
		c.Next()
	}
}
