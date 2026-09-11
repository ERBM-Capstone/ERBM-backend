// Package hash provides password hashing and verification helpers.
package hash

import "golang.org/x/crypto/bcrypt"

// HashPassword creates a bcrypt hash suitable for storing in User.PasswordHash.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// VerifyPassword returns nil when password matches a stored bcrypt hash.
func VerifyPassword(password, passwordHash string) error {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
}
