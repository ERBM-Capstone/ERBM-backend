package model

import (
	"encoding/json"
	"time"
)

// Employee is the local representation of an employee synchronized from Empeo.
type Employee struct {
	ID                 int64
	ExternalEmployeeID string
	Email              *string
	FirstName          string
	LastName           string
	FirstNameEN        *string
	LastNameEN         *string
	Nickname           *string
	NicknameEN         *string
	MobileNo           *string
	DateHired          *time.Time
	Position           *string
	EmploymentStatusID *int
	EmploymentStatus   *string
	IsActive           bool
	OrganizationLevel1 *string
	OrganizationLevel2 *string
	OrganizationLevel3 *string
	OrganizationLevel4 *string
	OrganizationLevel5 *string
	OrganizationLevel6 *string
	RawPayload         json.RawMessage
	LastSyncedAt       *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
