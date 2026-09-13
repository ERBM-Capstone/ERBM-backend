package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/gin-gonic/gin"
)

const (
	refreshCookieName = "refresh_token"
	refreshCookiePath = "/api/v1/auth"
)

type authService interface {
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, string, error)
	Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error)
}

// AuthHandler exposes manual authentication endpoints.
type AuthHandler struct {
	service             authService
	refreshTokenTTL     time.Duration
	refreshCookieSecure bool
}

// NewAuthHandler creates an AuthHandler backed by svc.
func NewAuthHandler(svc authService, refreshTokenTTL time.Duration, refreshCookieSecure bool) *AuthHandler {
	return &AuthHandler{
		service:             svc,
		refreshTokenTTL:     refreshTokenTTL,
		refreshCookieSecure: refreshCookieSecure,
	}
}

// Login godoc
// @Summary      Log in with email and password
// @Description  Verifies an active user's credentials, returns an access token, and sets an HttpOnly refresh cookie.
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

	response, refreshToken, err := h.service.Login(c.Request.Context(), &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	h.setRefreshCookie(c, refreshToken)
	c.JSON(http.StatusOK, response)
}

// Refresh godoc
// @Summary      Refresh an access token
// @Description  Validates the HttpOnly refresh cookie and returns a new access token.
// @Tags         auth
// @Produce      json
// @Param        refresh_token  cookie    string  true  "Refresh token"
// @Success      200            {object}  dto.LoginResponse
// @Failure      401            {object}  dto.ErrorResponse
// @Failure      500            {object}  dto.ErrorResponse
// @Router       /api/v1/auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie(refreshCookieName)
	if err != nil || refreshToken == "" {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	response, err := h.service.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// Logout godoc
// @Summary      Log out
// @Description  Clears the refresh-token cookie. Tokens are not revoked server-side.
// @Tags         auth
// @Success      204
// @Router       /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	h.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		MaxAge:   int(h.refreshTokenTTL / time.Second),
		HttpOnly: true,
		Secure:   h.refreshCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		HttpOnly: true,
		Secure:   h.refreshCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
