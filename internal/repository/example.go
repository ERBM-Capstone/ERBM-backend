package repository

import (
	"context"
	"errors"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExampleRepository handles all DB operations for the examples table.
type ExampleRepository struct {
	pool *pgxpool.Pool
}

// NewExampleRepository creates a new ExampleRepository backed by pool.
func NewExampleRepository(pool *pgxpool.Pool) *ExampleRepository {
	return &ExampleRepository{pool: pool}
}

// List returns all non-deleted examples ordered by most recently updated.
func (r *ExampleRepository) List(ctx context.Context) ([]model.Example, error) {
	const query = `
		SELECT id, name, description, status,
		       created_at, created_by,
		       updated_at, updated_by
		FROM   examples
		WHERE  deleted_at IS NULL
		ORDER  BY updated_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return []model.Example{}, err
	}
	defer rows.Close()

	results := []model.Example{}
	for rows.Next() {
		var e model.Example
		if err := rows.Scan(
			&e.ID, &e.Name, &e.Description, &e.Status,
			&e.CreatedAt, &e.CreatedBy,
			&e.UpdatedAt, &e.UpdatedBy,
		); err != nil {
			return []model.Example{}, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// GetByID retrieves a single non-deleted example by its UUID.
// Returns dto.ErrNotFound when no row is found.
func (r *ExampleRepository) GetByID(ctx context.Context, id string) (*model.Example, error) {
	const query = `
		SELECT id, name, description, status,
		       created_at, created_by,
		       updated_at, updated_by,
		       deleted_at, deleted_by
		FROM   examples
		WHERE  id = $1
		  AND  deleted_at IS NULL`

	var e model.Example
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&e.ID, &e.Name, &e.Description, &e.Status,
		&e.CreatedAt, &e.CreatedBy,
		&e.UpdatedAt, &e.UpdatedBy,
		&e.DeletedAt, &e.DeletedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, dto.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// Create inserts a new example row and returns the DB-populated record.
func (r *ExampleRepository) Create(ctx context.Context, item *model.Example, createdBy string) (*model.Example, error) {
	const query = `
		INSERT INTO examples (name, description, status, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING id, name, description, status,
		          created_at, created_by,
		          updated_at, updated_by`

	var e model.Example
	err := r.pool.QueryRow(ctx, query,
		item.Name, item.Description, item.Status, createdBy,
	).Scan(
		&e.ID, &e.Name, &e.Description, &e.Status,
		&e.CreatedAt, &e.CreatedBy,
		&e.UpdatedAt, &e.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// Update applies partial changes to an existing non-deleted example.
// The entire operation is wrapped in a transaction.
func (r *ExampleRepository) Update(ctx context.Context, item *model.Example, updatedBy string) (*model.Example, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const query = `
		UPDATE examples
		SET    name        = COALESCE(NULLIF($1, ''), name),
		       description = COALESCE($2, description),
		       status      = COALESCE(NULLIF($3::text, '')::example_status, status),
		       updated_by  = $4,
		       updated_at  = NOW()
		WHERE  id = $5
		  AND  deleted_at IS NULL
		RETURNING id, name, description, status,
		          created_at, created_by,
		          updated_at, updated_by`

	var e model.Example
	err = tx.QueryRow(ctx, query,
		item.Name, item.Description, string(item.Status), updatedBy, item.ID,
	).Scan(
		&e.ID, &e.Name, &e.Description, &e.Status,
		&e.CreatedAt, &e.CreatedBy,
		&e.UpdatedAt, &e.UpdatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, dto.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &e, nil
}

// Delete soft-deletes an example by setting deleted_at / deleted_by.
// Returns dto.ErrAlreadyDeleted when no row was affected (already deleted or missing).
func (r *ExampleRepository) Delete(ctx context.Context, id, deletedBy string) error {
	const query = `
		UPDATE examples
		SET    deleted_at = NOW(),
		       deleted_by = $2
		WHERE  id = $1
		  AND  deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, query, id, deletedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return dto.ErrAlreadyDeleted
	}
	return nil
}
