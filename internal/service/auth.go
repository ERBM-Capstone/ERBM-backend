package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	passwordhash "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/hash"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
)

type authRepository interface {
	GetUserByEmail(ctx context.Context, email string) (*model.User, string, error)
	GetUserByID(ctx context.Context, userID int64) (*model.User, string, error)
	UpdateLastLogin(ctx context.Context, userID int64) error
}

type tokenManager interface {
	GenerateAccessToken(userID int64, role string) (string, error)
	GenerateRefreshToken(userID int64) (string, error)
	ParseRefreshToken(encodedToken string) (*jwtmanager.RefreshClaims, error)
	AccessTokenTTL() time.Duration
}

// AuthService implements manual user authentication.
type AuthService struct {
	repo   authRepository
	tokens tokenManager
}

// NewAuthService creates an authentication service.
func NewAuthService(repo authRepository, tokens tokenManager) *AuthService {
	return &AuthService{repo: repo, tokens: tokens}
}

// Login verifies credentials and returns an access-token response plus a separate
// refresh token for the handler to store in an HttpOnly cookie.
func (s *AuthService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, string, error) {
	user, role, err := s.repo.GetUserByEmail(ctx, strings.TrimSpace(req.Email))
	if err != nil {
		if errors.Is(err, dto.ErrInvalidCredentials) {
			return nil, "", dto.ErrInvalidCredentials
		}
		return nil, "", dto.ErrInternalServer
	}

	if user.PasswordHash == nil {
		return nil, "", dto.ErrInvalidCredentials
	}
	if err := passwordhash.VerifyPassword(req.Password, *user.PasswordHash); err != nil {
		return nil, "", dto.ErrInvalidCredentials
	}
	if user.Status != model.UserStatusActive {
		return nil, "", dto.ErrInvalidCredentials
	}

	accessToken, err := s.tokens.GenerateAccessToken(user.ID, role)
	if err != nil {
		return nil, "", dto.ErrInternalServer
	}
	refreshToken, err := s.tokens.GenerateRefreshToken(user.ID)
	if err != nil {
		return nil, "", dto.ErrInternalServer
	}
	if err := s.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return nil, "", dto.ErrInternalServer
	}

	return newTokenResponse(accessToken, s.tokens.AccessTokenTTL()), refreshToken, nil
}

// Refresh validates a refresh token, reloads the account and current role, and
// returns a new access token without rotating the refresh token.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error) {
	claims, err := s.tokens.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil, dto.ErrUnauthorized
	}

	user, role, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, dto.ErrUnauthorized) {
			return nil, dto.ErrUnauthorized
		}
		return nil, dto.ErrInternalServer
	}
	if user.Status != model.UserStatusActive {
		return nil, dto.ErrUnauthorized
	}

	accessToken, err := s.tokens.GenerateAccessToken(user.ID, role)
	if err != nil {
		return nil, dto.ErrInternalServer
	}

	return newTokenResponse(accessToken, s.tokens.AccessTokenTTL()), nil
}

func newTokenResponse(accessToken string, ttl time.Duration) *dto.LoginResponse {
	return &dto.LoginResponse{
		AccessToken: accessToken,
		ExpiresIn:   int64(ttl / time.Second),
	}
}
