package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	contextKeyRequestID = "request_id"
	contextKeyUserID    = "user_id"
)

// SetUserID stores the authenticated user ID in Gin context.
func SetUserID(c *gin.Context, id uuid.UUID) {
	c.Set(contextKeyUserID, id)
}

// GetUserID extracts the authenticated user ID from Gin context.
func GetUserID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get(contextKeyUserID)
	if !exists {
		return uuid.Nil, false
	}
	id, ok := val.(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, false
	}
	return id, true
}

// GetRequestID extracts the request ID from Gin context.
func GetRequestID(c *gin.Context) string {
	val, exists := c.Get(contextKeyRequestID)
	if !exists {
		return ""
	}
	s, _ := val.(string)
	return s
}

func writeMiddlewareError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
