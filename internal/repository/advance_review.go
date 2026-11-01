package repository

import (
	"context"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
)

// Review persists PM-owned workflow metadata only. The caller locks the
// Project and Advance and writes its audit event in this same transaction.
func (r *AdvanceRepository) Review(ctx context.Context, tx *gorm.DB, advance *model.Advance) error {
	updates := map[string]any{
		"status": advance.Status, "updated_at": advance.UpdatedAt, "updated_by": advance.UpdatedBy,
	}
	if advance.Status == model.AdvanceStatusApproved {
		updates["approved_at"], updates["approved_by"] = advance.ApprovedAt, advance.ApprovedBy
	}
	result := tx.WithContext(ctx).Model(&model.Advance{}).Where("id = ?", advance.ID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return dto.ErrAdvanceNotFound
	}
	return nil
}
