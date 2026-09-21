package service

import (
	"context"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
)

type roleRepository interface {
	List(ctx context.Context) ([]model.Role, error)
}

// RoleService implements role lookup use cases.
type RoleService struct {
	repo roleRepository
}

// NewRoleService creates a RoleService backed by repo.
func NewRoleService(repo roleRepository) *RoleService {
	return &RoleService{repo: repo}
}

// List returns roles that can be assigned during employee provisioning.
func (s *RoleService) List(ctx context.Context) ([]dto.RoleResponse, error) {
	roles, err := s.repo.List(ctx)
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	return dto.ToRoleResponses(roles), nil
}
