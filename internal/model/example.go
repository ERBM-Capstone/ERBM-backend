package model

import "time"

// ExampleStatus represents the lifecycle state of an Example.
type ExampleStatus string

const (
	ExampleStatusActive   ExampleStatus = "active"
	ExampleStatusInactive ExampleStatus = "inactive"
)

// Example is the domain model mapped to the `examples` table.
type Example struct {
	ID          string
	Name        string
	Description *string
	Status      ExampleStatus
	CreatedAt   time.Time
	CreatedBy   *string
	UpdatedAt   time.Time  `json:"updated_at"`
	UpdatedBy   *string    `json:"updated_by,omitempty"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	DeletedBy   *string    `json:"deleted_by,omitempty"`
}
