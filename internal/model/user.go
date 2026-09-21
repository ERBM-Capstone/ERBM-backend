package model

import "time"

// UserStatus represents the application account lifecycle state.
type UserStatus string

const (
	UserStatusInvited     UserStatus = "INVITED"
	UserStatusActive      UserStatus = "ACTIVE"
	UserStatusDeactivated UserStatus = "DEACTIVATED"
)

// User is an ERBM account associated with a manually managed employee.
type User struct {
	ID                  int64
	EmployeeID          int64
	Email               string
	PasswordHash        *string
	RoleID              int
	Status              UserStatus
	FirstLogin          bool
	FailedLoginAttempts int
	LockedUntil         *time.Time
	LastLoginAt         *time.Time
	CreatedAt           time.Time
	CreatedBy           *int64
	UpdatedAt           time.Time
	UpdatedBy           *int64
}

// UserProfile is the read model returned for the authenticated user.
type UserProfile struct {
	UserID     int64
	EmployeeID int64
	Email      string
	Status     UserStatus
	FirstLogin bool
	RoleCode   string
	FirstName  string
	LastName   string
	Position   string
}
