package repository

import (
	"context"
	"errors"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// EmployeeRepository handles database operations for employees.
type EmployeeRepository struct {
	db *gorm.DB
}

const employeeBulkCreateBatchSize = 100

// NewEmployeeRepository creates an EmployeeRepository backed by db.
func NewEmployeeRepository(db *gorm.DB) *EmployeeRepository {
	return &EmployeeRepository{db: db}
}

// BulkCreate inserts employees using the caller's transaction. The transaction
// is deliberately not committed here so a service can atomically create the
// related user records in the same transaction.
func (r *EmployeeRepository) BulkCreate(ctx context.Context, tx *gorm.DB, employees []*model.Employee) error {
	if len(employees) == 0 {
		return nil
	}

	if err := tx.WithContext(ctx).CreateInBatches(employees, employeeBulkCreateBatchSize).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return dto.ErrEmployeeEmailExists
		}
		return err
	}

	return nil
}

// WithTx runs fn in a GORM transaction. GORM rolls back when fn returns an
// error or panics, and commits only after fn succeeds.
func (r *EmployeeRepository) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

func (r *EmployeeRepository) List(
	ctx context.Context,
	page int,
	pageSize int,
) ([]*model.Employee, int64, error) {
	var employees []*model.Employee
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Employee{})

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := db.
		Order("id ASC").
		Limit(pageSize).
		Offset(offset).
		Find(&employees).Error; err != nil {
		return nil, 0, err
	}

	return employees, total, nil
}

func (r *EmployeeRepository) GetByID(ctx context.Context, ID int64) (*model.Employee, error) {
	var employee model.Employee
	if err := r.db.WithContext(ctx).Where("id = ?", ID).Take(&employee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, dto.ErrNotFound
		}
		return nil, err
	}

	return &employee, nil
}

func (r *EmployeeRepository) Update(
	ctx context.Context,
	tx *gorm.DB,
	id int64,
	updates map[string]interface{},
) error {
	if err := tx.WithContext(ctx).
		Model(&model.Employee{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return dto.ErrEmployeeEmailExists
		}
		return err
	}

	return nil
}

func (r *EmployeeRepository) Deactivate(
	ctx context.Context,
	tx *gorm.DB,
	employeeID int64,
	endDate time.Time,
	userID int64,
) error {
	result := tx.WithContext(ctx).
		Model(&model.Employee{}).
		Where("id = ?", employeeID).
		Updates(map[string]any{
			"updated_by":          userID,
			"employment_end_date": endDate,
		})

	return result.Error
}
