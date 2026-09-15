package service

import (
	"context"
	"errors"
	"strings"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
)

type employeeRepository interface {
	BulkCreate(ctx context.Context, tx *gorm.DB, employees []*model.Employee) error
	WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error
}

type userRepository interface {
	BulkCreate(ctx context.Context, tx *gorm.DB, users []*model.User) error
	RoleIDsByCode(ctx context.Context, tx *gorm.DB) (map[string]int, error)
}

// EmployeeService contains employee business rules.
type EmployeeService struct {
	employeeRepo employeeRepository
	userRepo     userRepository
}

// NewEmployeeService creates an EmployeeService backed by repo.
func NewEmployeeService(employeeRepo employeeRepository, userRepo userRepository) *EmployeeService {
	return &EmployeeService{employeeRepo: employeeRepo, userRepo: userRepo}
}

// Create validates and normalizes employee data before persistence.
func (s *EmployeeService) Create(
	ctx context.Context,
	req []*model.Employee,
) ([]*model.Employee, error) {

	err := s.employeeRepo.WithTx(ctx, func(tx *gorm.DB) error {
		if err := s.employeeRepo.BulkCreate(ctx, tx, req); err != nil {
			if errors.Is(err, dto.ErrEmployeeEmailExists) {
				return dto.ErrEmployeeEmailExists
			}

			return dto.ErrInternalServer
		}

		roleIDs, err := s.userRepo.RoleIDsByCode(ctx, tx)
		if err != nil {
			return dto.ErrInternalServer
		}

		users := make([]*model.User, 0, len(req))
		for _, employee := range req {
			// A former employee must not receive a new login account.
			if !employee.IsActive() {
				continue
			}

			roleID, ok := roleIDs[roleCodeFromPosition(employee.Position)]
			if !ok {
				return dto.ErrInternalServer
			}
			users = append(users, dto.ToUser(employee.ID, employee.Email, roleID))
		}

		if err := s.userRepo.BulkCreate(ctx, tx, users); err != nil {
			if errors.Is(err, dto.ErrEmployeeEmailExists) {
				return dto.ErrEmployeeEmailExists
			}
			return dto.ErrInternalServer
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return req, nil
}

// var provisionedRoleCodes = []string{"EMPLOYEE", "ADMIN", "PC", "PM", "ACCOUNTING"}

func roleCodeFromPosition(position string) string {
	switch strings.ToUpper(strings.TrimSpace(position)) {
	case "ADMIN", "PC", "PM", "ACCOUNTING":
		return strings.ToUpper(strings.TrimSpace(position))
	default:
		return "EMPLOYEE"
	}
}
