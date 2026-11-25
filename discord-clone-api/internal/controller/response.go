package controller

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/logger"
)

// ErrorBody represents the inner error payload defined in openapi.yaml.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the standard error envelope returned by all API endpoints.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// WriteErrorStatus writes a JSON ErrorResponse with the provided HTTP status, code, and message.
func WriteErrorStatus(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
		},
	})
}

// WriteError maps a domain or unexpected error to its HTTP status and JSON ErrorResponse.
func WriteError(c *gin.Context, err error) {
	status, defaultCode, defaultMsg := mapSentinel(err)

	code := defaultCode
	message := defaultMsg

	var domErr *domain.Error
	if errors.As(err, &domErr) {
		if domErr.Code != "" {
			code = domErr.Code
		}
		if domErr.Message != "" {
			message = domErr.Message
		}
		if domErr.RetryAfter > 0 {
			seconds := max(int(math.Ceil(domErr.RetryAfter.Seconds())), 1)
			c.Header("Retry-After", strconv.Itoa(seconds))
		}
	}

	if status == http.StatusInternalServerError {
		logger.FromContext(c.Request.Context()).Error("unhandled server error", "err", err, "path", c.FullPath())
		code = "INTERNAL_ERROR"
		message = "an internal server error occurred"
	}

	WriteErrorStatus(c, status, code, message)
}

func mapSentinel(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return http.StatusBadRequest, "VALIDATION_ERROR", "invalid request payload"
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username/email or password"
	case errors.Is(err, domain.ErrInvalidToken):
		return http.StatusUnauthorized, "INVALID_TOKEN", "token is invalid or expired"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "UNAUTHORIZED", "authentication required"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "FORBIDDEN", "you do not have permission to perform this action"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND", "resource not found"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "CONFLICT", "resource already exists"
	case errors.Is(err, domain.ErrRateLimited):
		return http.StatusTooManyRequests, "RATE_LIMITED", "too many requests"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR", "an internal server error occurred"
	}
}
