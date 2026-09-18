package dto

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
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

// ── Shared response helpers ──────────────────────────────────────────────────

// ErrorResponse is the standard JSON error body.
type ErrorResponse struct {
	Error string `json:"error"`
}

// DeleteResponse is the standard JSON body returned on a soft-delete.
type DeleteResponse struct {
	Message string `json:"message"`
}

// HandleError writes the appropriate HTTP status and JSON body for err.
func HandleError(c *gin.Context, err error) {
	if status, ok := ErrorStatusMap[err]; ok {
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: ErrInternalServer.Error()})
}
