// ── Shared response helpers ──────────────────────────────────────────────────
package dto

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BaseResponse is the standard JSON envelope returned by ERBM APIs.
type BaseResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Errors  []any  `json:"errors,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
}

// Success writes a successful response using the standard envelope.
func Success(c *gin.Context, status int, data any, message string) {
	c.JSON(status, BaseResponse{Success: true, Message: message, Data: data})
}

// Error writes an error response using the standard envelope and aborts the
// remaining Gin handler chain.
func Error(c *gin.Context, status int, message string, errors ...any) {
	response := BaseResponse{Success: false, Message: message}
	if len(errors) > 0 {
		response.Errors = errors
	}
	c.AbortWithStatusJSON(status, response)
}

// OK writes a 200 response.
func OK(c *gin.Context, data any) { Success(c, http.StatusOK, data, "OK") }

// Created writes a 201 response.
func Created(c *gin.Context, data any) { Success(c, http.StatusCreated, data, "Created") }

// NoContent writes a 204 response without a body as required by HTTP.
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }

// WithMeta writes a successful response with pagination metadata.
func WithMeta(c *gin.Context, status int, data any, message string, meta *Meta) {
	c.JSON(status, BaseResponse{Success: true, Message: message, Data: data, Meta: meta})
}

// HandleError writes the appropriate HTTP status and JSON body for err.
func HandleError(c *gin.Context, err error) {
	if status, ok := ErrorStatusMap[err]; ok {
		Error(c, status, err.Error())
		return
	}
	Error(c, http.StatusInternalServerError, ErrInternalServer.Error())
}
