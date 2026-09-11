package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	passwordhash "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/hash"
)

type authRepository interface {
	GetUserByEmail(ctx context.Context, email string) (*model.User, string, error)
	UpdateLastLogin(ctx context.Context, userID int64) error
}

type accessTokenIssuer interface {
	GenerateAccessToken(userID int64, role string) (string, error)
	AccessTokenTTL() time.Duration
}

// AuthService implements manual user authentication.
type AuthService struct {
	repo   authRepository
	tokens accessTokenIssuer
}

// NewAuthService creates an authentication service.
func NewAuthService(repo authRepository, tokens accessTokenIssuer) *AuthService {
	return &AuthService{repo: repo, tokens: tokens}
}

// Login verifies credentials and returns a short-lived JWT access token.
func (s *AuthService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error) {
	user, role, err := s.repo.GetUserByEmail(ctx, strings.TrimSpace(req.Email))
	if err != nil {
		if errors.Is(err, dto.ErrInvalidCredentials) {
			return nil, dto.ErrInvalidCredentials
		}
		return nil, dto.ErrInternalServer
	}

	if user.PasswordHash == nil {
		return nil, dto.ErrInvalidCredentials
	}
	if err := passwordhash.VerifyPassword(req.Password, *user.PasswordHash); err != nil {
		return nil, dto.ErrInvalidCredentials
	}
	if user.Status != model.UserStatusActive {
		return nil, dto.ErrInvalidCredentials
	}

	accessToken, err := s.tokens.GenerateAccessToken(user.ID, role)
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	if err := s.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return nil, dto.ErrInternalServer
	}

	return &dto.LoginResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.tokens.AccessTokenTTL() / time.Second),
	}, nil
}
