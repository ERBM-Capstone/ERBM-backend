package service

import (
	"context"

	"github.com/your-org/project-hub-backend/dto"
	"github.com/your-org/project-hub-backend/internal/model"
	"github.com/your-org/project-hub-backend/internal/repository"
)

// ExampleService contains business logic for the Example resource.
type ExampleService struct {
	repo *repository.ExampleRepository
}

// NewExampleService creates a new ExampleService backed by repo.
func NewExampleService(repo *repository.ExampleRepository) *ExampleService {
	return &ExampleService{repo: repo}
}

// List returns all non-deleted examples.
func (s *ExampleService) List(ctx context.Context) ([]model.Example, error) {
	return s.repo.List(ctx)
}

// GetByID returns a single example by UUID or dto.ErrNotFound.
func (s *ExampleService) GetByID(ctx context.Context, id string) (*model.Example, error) {
	return s.repo.GetByID(ctx, id)
}

// Create validates, sets defaults, then persists a new example.
func (s *ExampleService) Create(ctx context.Context, req *dto.CreateExampleRequest, createdBy string) (*model.Example, error) {
	item := dto.ToCreateExample(req)
	// Business default: new items are always active.
	item.Status = model.ExampleStatusActive
	return s.repo.Create(ctx, item, createdBy)
}

// Update applies a partial update to an existing example.
func (s *ExampleService) Update(ctx context.Context, id string, req *dto.UpdateExampleRequest, updatedBy string) (*model.Example, error) {
	item := dto.ToUpdateExample(req)
	item.ID = id
	return s.repo.Update(ctx, item, updatedBy)
}

// Delete soft-deletes an example.
func (s *ExampleService) Delete(ctx context.Context, id, deletedBy string) error {
	return s.repo.Delete(ctx, id, deletedBy)
}
