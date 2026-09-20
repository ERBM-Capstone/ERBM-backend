package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
)

type employeeRepository interface {
	BulkCreate(ctx context.Context, tx *gorm.DB, employees []*model.Employee) error
	WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	List(ctx context.Context, page int, pageSize int) ([]*model.Employee, int64, error)
	GetByID(ctx context.Context, ID int64) (*model.Employee, error)
	Update(ctx context.Context, id int64, updates map[string]interface{}) error
	Deactivate(ctx context.Context, tx *gorm.DB, employeeID int64, endDate time.Time, actorID int64) error
}

type userRepository interface {
	BulkCreate(ctx context.Context, tx *gorm.DB, users []*model.User) error
	RoleIDsByCode(ctx context.Context, tx *gorm.DB) (map[string]int, error)
	Deactivate(ctx context.Context, tx *gorm.DB, employeeID int64, actorID int64) error
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

func (s *EmployeeService) GetByID(ctx context.Context, ID int64) (*model.Employee, error) {
	employee, err := s.employeeRepo.GetByID(ctx, ID)
	if err != nil {
		if errors.Is(err, dto.ErrNotFound) {
			return nil, dto.ErrNotFound
		}
		return nil, dto.ErrInternalServer
	}
	return employee, nil
}
func (s *EmployeeService) Update(ctx context.Context, employeeID int64, req *dto.UpdateEmployeeRequest, actorID int64) (*model.Employee, error) {
	_, err := s.employeeRepo.GetByID(ctx, employeeID)
	if err != nil {
		if errors.Is(err, dto.ErrNotFound) {
			return nil, dto.ErrNotFound
		}
		return nil, dto.ErrInternalServer
	}

	// The DTO mapper leaves omitted fields at their zero values. Build the map
	// selectively so PATCH never clears fields the client did not send. In
	// particular, date_hired is immutable and is never included here.
	updates := make(map[string]interface{})
	if req.FirstName != nil {
		updates["first_name"] = strings.TrimSpace(*req.FirstName)
	}
	if req.LastName != nil {
		updates["last_name"] = strings.TrimSpace(*req.LastName)
	}
	if req.Email != nil {
		updates["email"] = strings.ToLower(strings.TrimSpace(*req.Email))
	}
	if req.Position != nil {
		updates["position"] = strings.TrimSpace(*req.Position)
	}

	if len(updates) == 0 {
		return nil, dto.ErrBadRequest
	}

	updates["updated_by"] = actorID

	err = s.employeeRepo.Update(ctx, employeeID, updates)
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	updatedEmployee, err := s.employeeRepo.GetByID(ctx, employeeID)
	if err != nil {
		return nil, dto.ErrInternalServer
	}

	return updatedEmployee, nil

}

// func (s *EmployeeService) Deactivate(ctx context.Context, tx *gorm.DB, employeeID int64) error {

// 	return nil
// }

func (s *EmployeeService) Deactivate(
	ctx context.Context,
	actorID int64,
	employeeID int64,
	endDate string,
) error {
	if actorID <= 0 {
		return dto.ErrUnauthorized
	}

	employee, err := s.employeeRepo.GetByID(ctx, employeeID)
	if err != nil {
		return err
	}

	endDateParsed, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return dto.ErrBadRequest
	}

	if endDateParsed.Before(employee.DateHired) {
		return dto.ErrBadRequest
	}

	err = s.employeeRepo.WithTx(ctx, func(tx *gorm.DB) error {
		if err := s.employeeRepo.Deactivate(
			ctx, tx, employeeID, endDateParsed, actorID,
		); err != nil {
			return err
		}

		return s.userRepo.Deactivate(ctx, tx, employeeID, actorID)
	})
	if err != nil {
		return err
	}

	// เตรียม response เอง ไม่ต้อง query ซ้ำ

	return nil
}
