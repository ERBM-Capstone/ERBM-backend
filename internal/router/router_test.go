package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func TestLoginRouteIsPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := jwtmanager.NewManager("0123456789abcdef0123456789abcdef", "erbm", 15*time.Minute)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	router := New(nil, nil, tokens)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; login route may be behind authentication middleware", response.Code, http.StatusBadRequest)
	}
}
