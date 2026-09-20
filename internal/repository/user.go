package repository

import (
	"context"
	"errors"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const userBulkCreateBatchSize = 100

// UserRepository handles database operations for application users.
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a UserRepository backed by db.
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// BulkCreate inserts users using the caller's transaction. It is intended for
// provisioning users after their associated employees have been inserted.
func (r *UserRepository) BulkCreate(ctx context.Context, tx *gorm.DB, users []*model.User) error {
	if len(users) == 0 {
		return nil
	}

	if err := tx.WithContext(ctx).CreateInBatches(users, userBulkCreateBatchSize).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return dto.ErrEmployeeEmailExists
		}
		return err
	}

	return nil
}

// RoleIDsByCode loads IDs for the supplied role codes using the caller's
// transaction. Role IDs must be resolved from the database rather than
// hard-coded, so provisioning cannot accidentally grant the wrong role.
func (r *UserRepository) RoleIDsByCode(ctx context.Context, tx *gorm.DB) (map[string]int, error) {
	type roleRow struct {
		ID   int
		Code string
	}

	var roles []roleRow
	if err := tx.WithContext(ctx).
		Table("roles").
		Select("id, code").
		Find(&roles).Error; err != nil {
		return nil, err
	}

	roleIDs := make(map[string]int, len(roles))
	for _, role := range roles {
		roleIDs[role.Code] = role.ID
	}

	return roleIDs, nil
}

func (r *UserRepository) Deactivate(ctx context.Context, tx *gorm.DB, employeeID int64, actorID int64) error {
	result := tx.WithContext(ctx).
		Model(&model.User{}).
		Where("employee_id = ?", employeeID).
		Updates(map[string]any{
			"updated_by": actorID,
			"status":     model.UserStatusDeactivated,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return dto.ErrNotFound
	}

	return nil
}

func (r *UserRepository) UpdateEmail(ctx context.Context, tx *gorm.DB, employeeID int64, newEmail string, actorID int64) error {
	result := tx.WithContext(ctx).
		Model(&model.User{}).
		Where("employee_id = ?", employeeID).
		Updates(map[string]any{
			"updated_by": actorID,
			"email":      newEmail,
		})

	if result.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(result.Error, &pgErr) && pgErr.Code == "23505" {
			return dto.ErrEmployeeEmailExists
		}
		return result.Error
	}
	if result.RowsAffected == 0 {
		return dto.ErrNotFound
	}

	return nil
}
