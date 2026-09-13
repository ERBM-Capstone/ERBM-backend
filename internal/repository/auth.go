package repository

import (
	"context"
	"errors"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
)

// AuthRepository handles database operations required for authentication.
type AuthRepository struct {
	db *gorm.DB
}

// NewAuthRepository creates an AuthRepository backed by db.
func NewAuthRepository(db *gorm.DB) *AuthRepository {
	return &AuthRepository{db: db}
}

// GetUserByEmail returns a user and their role code for login verification.
func (r *AuthRepository) GetUserByEmail(ctx context.Context, email string) (*model.User, string, error) {
	type userWithRole struct {
		model.User
		RoleCode string
	}

	var result userWithRole
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`u.id, u.employee_id, u.email, u.password_hash,
			u.role_id, u.status, u.first_login,
			u.failed_login_attempts, u.locked_until, u.last_login_at,
			u.created_at, u.updated_at, r.code AS role_code`).
		Joins("JOIN roles r ON r.id = u.role_id").
		Where("u.email = ?", email).
		Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", dto.ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", err
	}

	return &result.User, result.RoleCode, nil
}

// GetUserByID returns a user and their current role code for token refresh.
func (r *AuthRepository) GetUserByID(ctx context.Context, userID int64) (*model.User, string, error) {
	type userWithRole struct {
		model.User
		RoleCode string
	}

	var result userWithRole
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`u.id, u.employee_id, u.email, u.password_hash,
			u.role_id, u.status, u.first_login,
			u.failed_login_attempts, u.locked_until, u.last_login_at,
			u.created_at, u.updated_at, r.code AS role_code`).
		Joins("JOIN roles r ON r.id = u.role_id").
		Where("u.id = ?", userID).
		Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", dto.ErrUnauthorized
	}
	if err != nil {
		return nil, "", err
	}

	return &result.User, result.RoleCode, nil
}

// RecordFailedLogin atomically increments a user's failed-login counter. An
// expired lock starts a new attempt window at one; reaching maxAttempts creates
// a new lock for lockDuration.
func (r *AuthRepository) RecordFailedLogin(
	ctx context.Context,
	userID int64,
	maxAttempts int,
	lockDuration time.Duration,
) error {
	const nextAttempts = `CASE
		WHEN locked_until IS NOT NULL AND locked_until <= NOW() THEN 1
		ELSE failed_login_attempts + 1
	END`
	const nextLockedUntil = `CASE
		WHEN (CASE
			WHEN locked_until IS NOT NULL AND locked_until <= NOW() THEN 1
			ELSE failed_login_attempts + 1
		END) >= ?
		THEN NOW() + (? * INTERVAL '1 second')
		ELSE NULL
	END`

	result := r.db.WithContext(ctx).
		Table("users").
		Where("id = ?", userID).
		Where("locked_until IS NULL OR locked_until <= NOW()").
		Updates(map[string]any{
			"failed_login_attempts": gorm.Expr(nextAttempts),
			"locked_until": gorm.Expr(
				nextLockedUntil,
				maxAttempts,
				int64(lockDuration/time.Second),
			),
			"updated_at": gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// UpdateLastLogin records a successful login and clears prior lockout state.
func (r *AuthRepository) UpdateLastLogin(ctx context.Context, userID int64) error {
	result := r.db.WithContext(ctx).
		Table("users").
		Where("id = ?", userID).
		Updates(map[string]any{
			"failed_login_attempts": 0,
			"last_login_at":         gorm.Expr("NOW()"),
			"locked_until":          nil,
			"updated_at":            gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return dto.ErrInvalidCredentials
	}

	return nil
}
