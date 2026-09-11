// Package jwt issues and validates ERBM JWT access tokens.
package jwt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

const (
	minimumSecretLength = 32
	tokenIDLength       = 16
)

var (
	ErrSecretTooShort = errors.New("jwt secret must be at least 32 bytes")
	ErrIssuerRequired = errors.New("jwt issuer is required")
	ErrTTLRequired    = errors.New("jwt access token TTL must be positive")
	ErrInvalidUserID  = errors.New("jwt user ID must be positive")
	ErrRoleRequired   = errors.New("jwt role is required")
	ErrInvalidToken   = errors.New("invalid jwt access token")
)

// Claims contains the authenticated identity carried by an ERBM access token.
type Claims struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	jwtlib.RegisteredClaims
}

// Manager signs and validates access tokens using HMAC SHA-256.
type Manager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

// NewManager creates an access-token manager.
func NewManager(secret, issuer string, ttl time.Duration) (*Manager, error) {
	if len(secret) < minimumSecretLength {
		return nil, ErrSecretTooShort
	}
	if strings.TrimSpace(issuer) == "" {
		return nil, ErrIssuerRequired
	}
	if ttl <= 0 {
		return nil, ErrTTLRequired
	}

	return &Manager{
		secret: []byte(secret),
		issuer: issuer,
		ttl:    ttl,
		now:    time.Now,
	}, nil
}

// AccessTokenTTL returns the configured lifetime of issued access tokens.
func (m *Manager) AccessTokenTTL() time.Duration {
	return m.ttl
}

// GenerateAccessToken signs a new access token for a user and role.
func (m *Manager) GenerateAccessToken(userID int64, role string) (string, error) {
	if userID <= 0 {
		return "", ErrInvalidUserID
	}

	role = strings.TrimSpace(role)
	if role == "" {
		return "", ErrRoleRequired
	}

	tokenID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("generate jwt token ID: %w", err)
	}

	issuedAt := m.now().UTC()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			ExpiresAt: jwtlib.NewNumericDate(issuedAt.Add(m.ttl)),
			IssuedAt:  jwtlib.NewNumericDate(issuedAt),
			ID:        tokenID,
		},
	}

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt access token: %w", err)
	}

	return signedToken, nil
}

// ParseAccessToken validates an access token and returns its trusted claims.
func (m *Manager) ParseAccessToken(encodedToken string) (*Claims, error) {
	if strings.TrimSpace(encodedToken) == "" {
		return nil, ErrInvalidToken
	}

	claims := &Claims{}
	token, err := jwtlib.ParseWithClaims(
		encodedToken,
		claims,
		func(token *jwtlib.Token) (any, error) {
			if token.Method != jwtlib.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing algorithm %q", token.Method.Alg())
			}
			return m.secret, nil
		},
		jwtlib.WithValidMethods([]string{jwtlib.SigningMethodHS256.Alg()}),
		jwtlib.WithIssuer(m.issuer),
		jwtlib.WithExpirationRequired(),
		jwtlib.WithIssuedAt(),
		jwtlib.WithTimeFunc(m.now),
	)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if err := m.validateClaims(claims); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	return claims, nil
}

func (m *Manager) validateClaims(claims *Claims) error {
	if claims.UserID <= 0 {
		return ErrInvalidUserID
	}
	if claims.Subject != strconv.FormatInt(claims.UserID, 10) {
		return errors.New("jwt subject does not match user ID")
	}
	if strings.TrimSpace(claims.Role) == "" {
		return ErrRoleRequired
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return errors.New("jwt timestamps are required")
	}
	if claims.ID == "" {
		return errors.New("jwt token ID is required")
	}

	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if lifetime <= 0 || lifetime > m.ttl {
		return errors.New("jwt token lifetime is invalid")
	}

	return nil
}

func newTokenID() (string, error) {
	random := make([]byte, tokenIDLength)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(random), nil
}
