package repository

import (
	"context"
	"errors"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AccountActivationTokenRepository struct{ db *gorm.DB }

func NewAccountActivationTokenRepository(db *gorm.DB) *AccountActivationTokenRepository {
	return &AccountActivationTokenRepository{db: db}
}

func (r *AccountActivationTokenRepository) WithTx(ctx context.Context, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(tx) })
}

func (r *AccountActivationTokenRepository) Create(ctx context.Context, tx *gorm.DB, token *model.AccountActivationToken) error {
	return tx.WithContext(ctx).Create(token).Error
}

func (r *AccountActivationTokenRepository) FindByHashForUpdate(ctx context.Context, tx *gorm.DB, hash string) (*model.AccountActivationToken, error) {
	var token model.AccountActivationToken
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hash).Take(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrNotFound
	}
	return &token, err
}

func (r *AccountActivationTokenRepository) FindUserByIDForUpdate(ctx context.Context, tx *gorm.DB, userID int64) (*model.User, error) {
	var user model.User
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrNotFound
	}
	return &user, err
}

func (r *AccountActivationTokenRepository) FindUserByEmployeeIDForUpdate(ctx context.Context, tx *gorm.DB, employeeID int64) (*model.User, error) {
	var user model.User
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("employee_id = ?", employeeID).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrNotFound
	}
	return &user, err
}

func (r *AccountActivationTokenRepository) FindUserByEmailForUpdate(ctx context.Context, tx *gorm.DB, email string) (*model.User, error) {
	var user model.User
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", email).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrNotFound
	}
	return &user, err
}

func (r *AccountActivationTokenRepository) InvalidateOutstanding(ctx context.Context, tx *gorm.DB, userID int64, now time.Time) error {
	return tx.WithContext(ctx).Model(&model.AccountActivationToken{}).
		Where("user_id = ? AND used_at IS NULL AND invalidated_at IS NULL", userID).
		Update("invalidated_at", now).Error
}

func (r *AccountActivationTokenRepository) MarkUsed(ctx context.Context, tx *gorm.DB, tokenID int64, now time.Time) error {
	result := tx.WithContext(ctx).Model(&model.AccountActivationToken{}).Where("id = ? AND used_at IS NULL", tokenID).Update("used_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return dto.ErrNotFound
	}
	return nil
}

func (r *AccountActivationTokenRepository) ActivateUser(ctx context.Context, tx *gorm.DB, userID int64, passwordHash string) error {
	result := tx.WithContext(ctx).Model(&model.User{}).Where("id = ? AND status = ?", userID, model.UserStatusInvited).Updates(map[string]any{
		"password_hash": passwordHash, "status": model.UserStatusActive, "updated_at": time.Now(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return dto.ErrNotFound
	}
	return nil
}
