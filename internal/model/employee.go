package model

import "time"

// Employee contains manually managed employment and profile information.
type Employee struct {
	ID                int64
	FirstName         string
	LastName          string
	Position          string
	Email             string
	DateHired         time.Time
	EmploymentEndDate *time.Time
	CreatedAt         time.Time
	CreatedBy         *int64
	UpdatedAt         time.Time
	UpdatedBy         *int64
}

// IsActive derives employment activity from the confirmed end-date rule.
func (e Employee) IsActive() bool {
	return e.EmploymentEndDate == nil
}
