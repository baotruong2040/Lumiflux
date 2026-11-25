package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors representing domain-level failure categories.
var (
	ErrValidation         = errors.New("validation error")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrForbidden          = errors.New("forbidden")
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
	ErrRateLimited        = errors.New("rate limited")
	ErrInternal           = errors.New("internal error")
)

// Error pairs a machine-readable API error code and message with a sentinel error.
type Error struct {
	Code    string
	Message string
	Err     error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying sentinel error for errors.Is/errors.As matching.
func (e *Error) Unwrap() error {
	return e.Err
}

// NewError creates a domain Error wrapping a sentinel category.
func NewError(sentinel error, code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Err:     sentinel,
	}
}
