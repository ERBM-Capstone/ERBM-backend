package dto

import "github.com/ERBM-Capstone/ERBM-backend/internal/model"

// ToUser creates an invited user for an employee with a confirmed role.
// PasswordHash is deliberately left nil until the user completes OTP activation.
func ToUser(employeeID int64, email string, roleID int) *model.User {
	return &model.User{
		EmployeeID: employeeID,
		Email:      email,
		RoleID:     roleID,
		Status:     model.UserStatusInvited,
		FirstLogin: true,
	}
}
