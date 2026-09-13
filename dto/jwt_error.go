package dto

import "errors"

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
