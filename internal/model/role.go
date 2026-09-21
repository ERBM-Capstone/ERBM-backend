package model

import "time"

// Role is an application permission role.
type Role struct {
	ID        int
	Code      string
	Name      string
	CreatedAt time.Time
}
