// Package jwt issues and validates ERBM JWT access and refresh tokens.
package jwt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

const (
	minimumSecretLength = 32
	tokenIDLength       = 16

	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// Claims contains the authenticated identity carried by an ERBM access token.
type Claims struct {
	UserID    int64  `json:"user_id"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	jwtlib.RegisteredClaims
}

// RefreshClaims contains only the identity required to obtain a new access token.
// The current role is deliberately loaded from PostgreSQL during refresh.
type RefreshClaims struct {
	UserID    int64  `json:"user_id"`
	TokenType string `json:"token_type"`
	jwtlib.RegisteredClaims
}

// Manager signs and validates access and refresh tokens using separate HMAC secrets.
type Manager struct {
	accessSecret  []byte
	refreshSecret []byte
	issuer        string
	accessTTL     time.Duration
	refreshTTL    time.Duration
	now           func() time.Time
}

// NewManager creates an access- and refresh-token manager.
func NewManager(accessSecret, refreshSecret, issuer string, accessTTL, refreshTTL time.Duration) (*Manager, error) {
	if len(accessSecret) < minimumSecretLength || len(refreshSecret) < minimumSecretLength {
		return nil, dto.ErrSecretTooShort
	}
	if accessSecret == refreshSecret {
		return nil, dto.ErrSecretsMustDiffer
	}
	if strings.TrimSpace(issuer) == "" {
		return nil, dto.ErrIssuerRequired
	}
	if accessTTL <= 0 {
		return nil, dto.ErrAccessTTLRequired
	}
	if refreshTTL <= 0 {
		return nil, dto.ErrRefreshTTLRequired
	}

	return &Manager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		issuer:        issuer,
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
		now:           time.Now,
	}, nil
}

// AccessTokenTTL returns the configured lifetime of issued access tokens.
func (m *Manager) AccessTokenTTL() time.Duration {
	return m.accessTTL
}

// RefreshTokenTTL returns the configured lifetime of issued refresh tokens.
func (m *Manager) RefreshTokenTTL() time.Duration {
	return m.refreshTTL
}

// GenerateAccessToken signs a new access token for a user and role.
func (m *Manager) GenerateAccessToken(userID int64, role string) (string, error) {
	if userID <= 0 {
		return "", dto.ErrInvalidUserID
	}

	role = strings.TrimSpace(role)
	if role == "" {
		return "", dto.ErrRoleRequired
	}

	registeredClaims, err := m.newRegisteredClaims(userID, m.accessTTL)
	if err != nil {
		return "", fmt.Errorf("generate jwt access token claims: %w", err)
	}

	claims := Claims{
		UserID:           userID,
		Role:             role,
		TokenType:        TokenTypeAccess,
		RegisteredClaims: registeredClaims,
	}

	return sign(claims, m.accessSecret, "access")
}

// GenerateRefreshToken signs a new refresh token containing no role claim.
func (m *Manager) GenerateRefreshToken(userID int64) (string, error) {
	if userID <= 0 {
		return "", dto.ErrInvalidUserID
	}

	registeredClaims, err := m.newRegisteredClaims(userID, m.refreshTTL)
	if err != nil {
		return "", fmt.Errorf("generate jwt refresh token claims: %w", err)
	}

	claims := RefreshClaims{
		UserID:           userID,
		TokenType:        TokenTypeRefresh,
		RegisteredClaims: registeredClaims,
	}

	return sign(claims, m.refreshSecret, "refresh")
}

// ParseAccessToken validates an access token and returns its trusted claims.
func (m *Manager) ParseAccessToken(encodedToken string) (*Claims, error) {
	if strings.TrimSpace(encodedToken) == "" {
		return nil, dto.ErrInvalidAccessToken
	}

	claims := &Claims{}
	if err := m.parse(encodedToken, claims, m.accessSecret); err != nil {
		return nil, fmt.Errorf("%w: %v", dto.ErrInvalidAccessToken, err)
	}
	if err := validateRegisteredClaims(claims.UserID, claims.RegisteredClaims, m.accessTTL); err != nil {
		return nil, fmt.Errorf("%w: %v", dto.ErrInvalidAccessToken, err)
	}
	if claims.TokenType != TokenTypeAccess {
		return nil, fmt.Errorf("%w: unexpected token type", dto.ErrInvalidAccessToken)
	}
	if strings.TrimSpace(claims.Role) == "" {
		return nil, fmt.Errorf("%w: %v", dto.ErrInvalidAccessToken, dto.ErrRoleRequired)
	}

	return claims, nil
}

// ParseRefreshToken validates a refresh token and returns its trusted claims.
func (m *Manager) ParseRefreshToken(encodedToken string) (*RefreshClaims, error) {
	if strings.TrimSpace(encodedToken) == "" {
		return nil, dto.ErrInvalidRefreshToken
	}

	claims := &RefreshClaims{}
	if err := m.parse(encodedToken, claims, m.refreshSecret); err != nil {
		return nil, fmt.Errorf("%w: %v", dto.ErrInvalidRefreshToken, err)
	}
	if err := validateRegisteredClaims(claims.UserID, claims.RegisteredClaims, m.refreshTTL); err != nil {
		return nil, fmt.Errorf("%w: %v", dto.ErrInvalidRefreshToken, err)
	}
	if claims.TokenType != TokenTypeRefresh {
		return nil, fmt.Errorf("%w: unexpected token type", dto.ErrInvalidRefreshToken)
	}

	return claims, nil
}

func (m *Manager) newRegisteredClaims(userID int64, ttl time.Duration) (jwtlib.RegisteredClaims, error) {
	tokenID, err := newTokenID()
	if err != nil {
		return jwtlib.RegisteredClaims{}, err
	}

	issuedAt := m.now().UTC()
	return jwtlib.RegisteredClaims{
		Issuer:    m.issuer,
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: jwtlib.NewNumericDate(issuedAt.Add(ttl)),
		IssuedAt:  jwtlib.NewNumericDate(issuedAt),
		ID:        tokenID,
	}, nil
}

func (m *Manager) parse(encodedToken string, claims jwtlib.Claims, secret []byte) error {
	token, err := jwtlib.ParseWithClaims(
		encodedToken,
		claims,
		func(token *jwtlib.Token) (any, error) {
			if token.Method != jwtlib.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing algorithm %q", token.Method.Alg())
			}
			return secret, nil
		},
		jwtlib.WithValidMethods([]string{jwtlib.SigningMethodHS256.Alg()}),
		jwtlib.WithIssuer(m.issuer),
		jwtlib.WithExpirationRequired(),
		jwtlib.WithIssuedAt(),
		jwtlib.WithTimeFunc(m.now),
	)
	if err != nil {
		return err
	}
	if !token.Valid {
		return errors.New("token is not valid")
	}

	return nil
}

func validateRegisteredClaims(userID int64, claims jwtlib.RegisteredClaims, maximumTTL time.Duration) error {
	if userID <= 0 {
		return dto.ErrInvalidUserID
	}
	if claims.Subject != strconv.FormatInt(userID, 10) {
		return errors.New("jwt subject does not match user ID")
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return errors.New("jwt timestamps are required")
	}
	if claims.ID == "" {
		return errors.New("jwt token ID is required")
	}

	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if lifetime <= 0 || lifetime > maximumTTL {
		return errors.New("jwt token lifetime is invalid")
	}

	return nil
}

func sign(claims jwtlib.Claims, secret []byte, tokenType string) (string, error) {
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt %s token: %w", tokenType, err)
	}

	return signedToken, nil
}

func newTokenID() (string, error) {
	random := make([]byte, tokenIDLength)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(random), nil
}
