package dto

import (
	"errors"
	"net/http"
)

// ── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound            = errors.New("not found")
	ErrAlreadyDeleted      = errors.New("already deleted")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrBadRequest          = errors.New("bad request")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrAccountLocked       = errors.New("account temporarily locked")
	ErrInternalServer      = errors.New("internal server error")
	ErrEmployeeEmailExists = errors.New("employee email already exists")

	// Example resource
	ErrExampleNotFound = errors.New("example not found")
)
var (
	ErrSecretTooShort      = errors.New("jwt secret must be at least 32 bytes")
	ErrSecretsMustDiffer   = errors.New("jwt access and refresh secrets must differ")
	ErrIssuerRequired      = errors.New("jwt issuer is required")
	ErrAccessTTLRequired   = errors.New("jwt access token TTL must be positive")
	ErrRefreshTTLRequired  = errors.New("jwt refresh token TTL must be positive")
	ErrInvalidUserID       = errors.New("jwt user ID must be positive")
	ErrRoleRequired        = errors.New("jwt role is required")
	ErrInvalidAccessToken  = errors.New("invalid jwt access token")
	ErrInvalidRefreshToken = errors.New("invalid jwt refresh token")
)

// ── Error → HTTP status map ──────────────────────────────────────────────────

// ErrorStatusMap maps sentinel errors to HTTP status codes.
var ErrorStatusMap = map[error]int{
	ErrNotFound:            http.StatusNotFound,
	ErrAlreadyDeleted:      http.StatusGone,
	ErrUnauthorized:        http.StatusUnauthorized,
	ErrForbidden:           http.StatusForbidden,
	ErrBadRequest:          http.StatusBadRequest,
	ErrInvalidCredentials:  http.StatusUnauthorized,
	ErrAccountLocked:       http.StatusLocked,
	ErrInternalServer:      http.StatusInternalServerError,
	ErrEmployeeEmailExists: http.StatusConflict,
	ErrExampleNotFound:     http.StatusNotFound,
}
