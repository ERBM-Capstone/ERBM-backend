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
	List(ctx context.Context, page int, pageSize int) ([]*model.Employee, int64, error)
}

type userRepository interface {
	BulkCreate(ctx context.Context, tx *gorm.DB, users []*model.User) error
	RoleIDsByCode(ctx context.Context, tx *gorm.DB) (map[string]int, error)
}

// EmployeeProvision pairs HR employee data with the application role assigned
// to that employee's invited account. Position is not used for authorization.
type EmployeeProvision struct {
	Employee *model.Employee
	RoleCode string
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
	actorID int64,
	req []EmployeeProvision,
) ([]*model.Employee, error) {
	if actorID <= 0 || len(req) == 0 {
		return nil, dto.ErrBadRequest
	}

	roleCodes := make([]string, len(req))
	employees := make([]*model.Employee, len(req))
	for i, provision := range req {
		if provision.Employee == nil {
			return nil, dto.ErrBadRequest
		}
		roleCode := strings.ToUpper(strings.TrimSpace(provision.RoleCode))
		if roleCode == "" {
			return nil, dto.ErrBadRequest
		}
		roleCodes[i] = roleCode
		provision.Employee.CreatedBy = &actorID
		provision.Employee.UpdatedBy = &actorID
		employees[i] = provision.Employee
	}

	err := s.employeeRepo.WithTx(ctx, func(tx *gorm.DB) error {
		roleIDs, err := s.userRepo.RoleIDsByCode(ctx, tx)
		if err != nil {
			return dto.ErrInternalServer
		}
		roleIDsByProvision := make([]int, len(req))
		for i, roleCode := range roleCodes {
			roleID, ok := roleIDs[roleCode]
			if !ok {
				return dto.ErrBadRequest
			}
			roleIDsByProvision[i] = roleID
		}

		if err := s.employeeRepo.BulkCreate(ctx, tx, employees); err != nil {
			if errors.Is(err, dto.ErrEmployeeEmailExists) {
				return dto.ErrEmployeeEmailExists
			}

			return dto.ErrInternalServer
		}

		users := make([]*model.User, 0, len(req))
		for i, employee := range employees {
			// A former employee must not receive a new login account.
			if !employee.IsActive() {
				continue
			}

			users = append(users, dto.ToUser(employee.ID, employee.Email, roleIDsByProvision[i], actorID))
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

	return employees, nil
}

func (s *EmployeeService) List(ctx context.Context, page int, pageSize int) ([]*model.Employee, int64, error) {
	employees, total, err := s.employeeRepo.List(ctx, page, pageSize)
	if err != nil {
		return nil, 0, dto.ErrInternalServer
	}
	return employees, total, nil
}
