package dto

import "github.com/ERBM-Capstone/ERBM-backend/internal/model"

// LoginRequest contains the credentials for a manual user login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse contains a bearer access token and its lifetime in seconds.
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

// MeResponse contains the authenticated account and its employee profile.
type MeResponse struct {
	UserID     int64            `json:"user_id"`
	EmployeeID int64            `json:"employee_id"`
	Email      string           `json:"email"`
	Status     model.UserStatus `json:"status"`
	FirstLogin bool             `json:"first_login"`
	RoleCode   string           `json:"role_code"`
	FirstName  string           `json:"first_name"`
	LastName   string           `json:"last_name"`
	Position   string           `json:"position"`
}

// ToMeResponse converts the authentication read model to the public response.
func ToMeResponse(profile *model.UserProfile) MeResponse {
	return MeResponse{
		UserID:     profile.UserID,
		EmployeeID: profile.EmployeeID,
		Email:      profile.Email,
		Status:     profile.Status,
		FirstLogin: profile.FirstLogin,
		RoleCode:   profile.RoleCode,
		FirstName:  profile.FirstName,
		LastName:   profile.LastName,
		Position:   profile.Position,
	}
}

// RoleResponse is a role that can be assigned to a user.
type RoleResponse struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// ToRoleResponses converts roles to their public representation.
func ToRoleResponses(roles []model.Role) []RoleResponse {
	response := make([]RoleResponse, 0, len(roles))
	for _, role := range roles {
		response = append(response, RoleResponse{ID: role.ID, Code: role.Code, Name: role.Name})
	}
	return response
}
