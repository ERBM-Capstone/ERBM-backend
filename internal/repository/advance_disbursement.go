package repository

import (
	"context"
	"errors"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// HasActiveAdvance is checked while the caller holds the Project row lock.
// Sharing that lock across activations prevents concurrent disbursements from
// opening two Advances for the same Project. The partial unique index is the
// final database guard against writers outside that lock protocol.
func (r *AdvanceRepository) HasActiveAdvance(ctx context.Context, tx *gorm.DB, projectID int64) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).Model(&model.Advance{}).
		Where("project_id = ? AND status = ?", projectID, model.AdvanceStatusActive).
		Count(&count).Error
	return count > 0, err
}

func (r *AdvanceRepository) CreateDisbursement(ctx context.Context, tx *gorm.DB, payment *model.AdvanceDisbursement) error {
	err := tx.WithContext(ctx).Create(payment).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_advance_disbursements_advance" {
		return dto.ErrAdvanceAlreadyDisbursed
	}
	return err
}

// Activate writes only Accounting-owned workflow metadata. The service holds
// Project -> Advance locks and records payment and audit in this transaction.
func (r *AdvanceRepository) Activate(ctx context.Context, tx *gorm.DB, advance *model.Advance) error {
	result := tx.WithContext(ctx).Model(&model.Advance{}).Where("id = ?", advance.ID).Updates(map[string]any{
		"status": advance.Status, "activated_at": advance.ActivatedAt,
		"updated_by": advance.UpdatedBy, "updated_at": advance.UpdatedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(result.Error, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_advances_one_active_per_project" {
		return dto.ErrProjectHasActiveAdvance
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return dto.ErrAdvanceNotFound
	}
	return nil
}
