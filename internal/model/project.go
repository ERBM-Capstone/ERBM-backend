package model

import "time"

// ProjectStatus represents the lifecycle state of a project.
type ProjectStatus string

const (
	ProjectStatusActive ProjectStatus = "ACTIVE"
	ProjectStatusClosed ProjectStatus = "CLOSED"
)

// Project groups the funds of a project. Budget is stored as a decimal string
// so the NUMERIC value from PostgreSQL is never rounded through float64.
type Project struct {
	ID          int64
	ProjectCode string
	Name        string
	Description *string
	Budget      string
	StartDate   *time.Time
	EndDate     *time.Time
	Status      ProjectStatus
	PMUserID    int64 `gorm:"column:pm_user_id"`
	ClosedAt    *time.Time
	ClosedBy    *int64
	CreatedAt   time.Time
	CreatedBy   *int64
	UpdatedAt   time.Time
	UpdatedBy   *int64
}

// ProjectCoordinator links a PC user to a project.
type ProjectCoordinator struct {
	ProjectID int64
	UserID    int64
	CreatedBy *int64
	CreatedAt time.Time
}

// ProjectMember is the read model of a PM or PC shown with a project.
type ProjectMember struct {
	ProjectID int64
	UserID    int64
	FirstName string
	LastName  string
	Email     string
}

// ProjectDetail is a project together with its PM and PCs.
type ProjectDetail struct {
	Project *Project
	PM      *ProjectMember
	PCs     []ProjectMember
}

// AssignableUser is the account data needed to check a PM/PC assignment.
type AssignableUser struct {
	UserID   int64
	RoleCode string
	Status   UserStatus
}

// ProjectFilter holds list filters. MemberUserID limits the result to
// projects where that user is the PM or one of the PCs.
type ProjectFilter struct {
	MemberUserID *int64
	Query        string
	Status       string
	Page         int
	PageSize     int
}
