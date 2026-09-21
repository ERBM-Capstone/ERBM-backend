package repository

import (
	"context"

	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
)

// RoleRepository handles read access to application roles.
type RoleRepository struct {
	db *gorm.DB
}

// NewRoleRepository creates a RoleRepository backed by db.
func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

// List returns assignable roles in stable ID order.
func (r *RoleRepository) List(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}
