package dto

import (
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
)

// ── URI param ────────────────────────────────────────────────────────────────

// ExampleID is bound from the /:id path parameter.
type ExampleID struct {
	ID string `uri:"id" binding:"required,uuid"`
}

// ── Request bodies ───────────────────────────────────────────────────────────

// CreateExampleRequest is the JSON body for POST /api/v1/examples.
type CreateExampleRequest struct {
	Name        string  `json:"name"        binding:"required,min=1,max=255"`
	Description *string `json:"description" binding:"omitempty,max=1000"`
}

// UpdateExampleRequest is the JSON body for PUT /api/v1/examples/:id.
type UpdateExampleRequest struct {
	Name        *string              `json:"name"        binding:"omitempty,min=1,max=255"`
	Description *string              `json:"description" binding:"omitempty,max=1000"`
	Status      *model.ExampleStatus `json:"status"      binding:"omitempty,oneof=active inactive"`
}

// ── Response ─────────────────────────────────────────────────────────────────

// ExampleResponse is the public-facing shape returned by the API.
type ExampleResponse struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description *string             `json:"description,omitempty"`
	Status      model.ExampleStatus `json:"status"`
	CreatedAt   string              `json:"created_at"`
	CreatedBy   *string             `json:"created_by,omitempty"`
	UpdatedAt   string              `json:"updated_at"`
}

// ── Mappers ──────────────────────────────────────────────────────────────────

// ToCreateExample maps a CreateExampleRequest to a model.Example ready for insertion.
func ToCreateExample(params *CreateExampleRequest) *model.Example {
	return &model.Example{
		Name:        params.Name,
		Description: params.Description,
		Status:      model.ExampleStatusActive,
	}
}

// ToUpdateExample maps an UpdateExampleRequest to a partial model.Example.
func ToUpdateExample(params *UpdateExampleRequest) *model.Example {
	e := &model.Example{}
	if params.Name != nil {
		e.Name = *params.Name
	}
	if params.Description != nil {
		e.Description = params.Description
	}
	if params.Status != nil {
		e.Status = *params.Status
	}
	return e
}

// ToExampleResponse converts a domain model to the API response shape.
func ToExampleResponse(item *model.Example) ExampleResponse {
	return ExampleResponse{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		Status:      item.Status,
		CreatedAt:   item.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedBy:   item.CreatedBy,
		UpdatedAt:   item.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
