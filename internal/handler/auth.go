package handler

import (
	"context"
	"net/http"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/gin-gonic/gin"
)

type authService interface {
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error)
}

// AuthHandler exposes manual authentication endpoints.
type AuthHandler struct {
	service authService
}

// NewAuthHandler creates an AuthHandler backed by svc.
func NewAuthHandler(svc authService) *AuthHandler {
	return &AuthHandler{service: svc}
}

// Login godoc
// @Summary      Log in with email and password
// @Description  Verifies an active user's credentials and returns a JWT access token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      dto.LoginRequest  true  "Login credentials"
// @Success      200   {object}  dto.LoginResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      401   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	response, err := h.service.Login(c.Request.Context(), &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}
