// Package activation creates and hashes account-activation tokens.
package activation

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const TokenTTL = 24 * time.Hour

// Generate returns a URL-safe plaintext token, its SHA-256 hash, and expiry.
func Generate(now time.Time) (string, string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", time.Time{}, err
	}

	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	return plaintext, Hash(plaintext), now.Add(TokenTTL), nil
}

// Hash returns the database-safe digest of a plaintext activation token.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
