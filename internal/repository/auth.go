package repository

import (
	"context"
	"errors"

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

// UpdateLastLogin records a successful login for a user.
func (r *AuthRepository) UpdateLastLogin(ctx context.Context, userID int64) error {
	result := r.db.WithContext(ctx).
		Table("users").
		Where("id = ?", userID).
		Updates(map[string]any{
			"last_login_at": gorm.Expr("NOW()"),
			"updated_at":    gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return dto.ErrInvalidCredentials
	}

	return nil
}
