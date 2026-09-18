package repository

import (
	"context"
	"errors"

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
