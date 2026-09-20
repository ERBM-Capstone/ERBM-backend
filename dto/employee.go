package dto

import (
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
)

const employeeDateFormat = "2006-01-02"

// CreateEmployeeRequest is the JSON body for POST /api/v1/employees.
type CreateEmployeeRequest struct {
	FirstName         string  `json:"first_name" binding:"required,max=255"`
	LastName          string  `json:"last_name" binding:"required,max=255"`
	Position          string  `json:"position" binding:"required,max=255"`
	Email             string  `json:"email" binding:"required,email,max=255"`
	DateHired         string  `json:"date_hired" binding:"required,datetime=2006-01-02"`
	RoleCode          string  `json:"role_code" binding:"required,max=50"`
	EmploymentEndDate *string `json:"employment_end_date" binding:"omitempty,datetime=2006-01-02"`
}
type UpdateEmployeeRequest struct {
	FirstName *string `json:"first_name" binding:"omitempty,max=255"`
	LastName  *string `json:"last_name" binding:"omitempty,max=255"`
	Position  *string `json:"position" binding:"omitempty,max=255"`
	Email     *string `json:"email" binding:"omitempty,email,max=255"`
}
type DeactivateEmployeeRequest struct {
	EmploymentEndDate string `json:"employment_end_date" binding:"required,datetime=2006-01-02"`
}

// ToEmployee converts a validated employee request into its persistence model.
// Role and account data intentionally belong to User and are not set here.
func ToEmployee(req *CreateEmployeeRequest) (*model.Employee, error) {
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	position := strings.TrimSpace(req.Position)
	email := strings.ToLower(strings.TrimSpace(req.Email))

	dateHired, err := time.Parse(employeeDateFormat, req.DateHired)
	if err != nil {
		return nil, err
	}

	var employmentEndDate *time.Time

	if req.EmploymentEndDate != nil {
		parsed, err := time.Parse(employeeDateFormat, *req.EmploymentEndDate)
		if err != nil {
			return nil, err
		}
		employmentEndDate = &parsed
	}
	if employmentEndDate != nil && employmentEndDate.Before(dateHired) {
		return nil, ErrBadRequest
	}

	return &model.Employee{
		FirstName:         firstName,
		LastName:          lastName,
		Position:          position,
		Email:             email,
		DateHired:         dateHired,
		EmploymentEndDate: employmentEndDate,
	}, nil
}

// EmployeeResponse is the public-facing shape returned by the employee API.
type EmployeeResponse struct {
	ID                int64   `json:"id"`
	FirstName         string  `json:"first_name"`
	LastName          string  `json:"last_name"`
	Position          string  `json:"position"`
	Email             string  `json:"email"`
	DateHired         string  `json:"date_hired"`
	EmploymentEndDate *string `json:"employment_end_date,omitempty"`
	IsActive          bool    `json:"is_active"`
}

// ToEmployeeResponse converts an employee model to its API response shape.
func ToEmployeeResponse(employee *model.Employee) EmployeeResponse {
	var employmentEndDate *string
	if employee.EmploymentEndDate != nil {
		formatted := employee.EmploymentEndDate.Format(employeeDateFormat)
		employmentEndDate = &formatted
	}

	return EmployeeResponse{
		ID:                employee.ID,
		FirstName:         employee.FirstName,
		LastName:          employee.LastName,
		Position:          employee.Position,
		Email:             employee.Email,
		DateHired:         employee.DateHired.Format(employeeDateFormat),
		EmploymentEndDate: employmentEndDate,
		IsActive:          employee.IsActive(),
	}
}
