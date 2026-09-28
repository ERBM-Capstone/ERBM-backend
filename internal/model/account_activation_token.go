package model

import "time"

// AccountActivationToken is a single-use invitation token. TokenHash is a
// SHA-256 digest; plaintext activation tokens are never persisted.
type AccountActivationToken struct {
	ID            int64
	UserID        int64
	TokenHash     string
	ExpiresAt     time.Time
	UsedAt        *time.Time
	InvalidatedAt *time.Time
	CreatedAt     time.Time
}
