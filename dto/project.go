package dto

import "github.com/ERBM-Capstone/ERBM-backend/internal/model"

const projectDateFormat = "2006-01-02"

// CreateProjectRequest is the JSON body for POST /api/v1/projects.
// Budget is a decimal string such as "1000.00".
type CreateProjectRequest struct {
	ProjectCode string  `json:"project_code" binding:"required,max=50"`
	Name        string  `json:"name" binding:"required,max=255"`
	Description *string `json:"description"`
	Budget      string  `json:"budget" binding:"required"`
	StartDate   *string `json:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     *string `json:"end_date" binding:"omitempty,datetime=2006-01-02"`
	PMUserID    *int64  `json:"pm_user_id" binding:"omitempty,min=1"`
	PCUserIDs   []int64 `json:"pc_user_ids" binding:"omitempty,dive,min=1"`
}

// UpdateProjectRequest is the JSON body for PATCH /api/v1/projects/{id}.
// Omitted fields are left unchanged. Send status "CLOSED" to close the project.
type UpdateProjectRequest struct {
	Name        *string  `json:"name" binding:"omitempty,max=255"`
	Description *string  `json:"description"`
	Budget      *string  `json:"budget"`
	StartDate   *string  `json:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     *string  `json:"end_date" binding:"omitempty,datetime=2006-01-02"`
	PMUserID    *int64   `json:"pm_user_id" binding:"omitempty,min=1"`
	PCUserIDs   *[]int64 `json:"pc_user_ids" binding:"omitempty,dive,min=1"`
	Status      *string  `json:"status" binding:"omitempty,oneof=CLOSED"`
}

// ProjectListQuery contains pagination and filters for GET /api/v1/projects.
type ProjectListQuery struct {
	ListQuery
	Q      string `form:"q" binding:"omitempty,max=255"`
	Status string `form:"status" binding:"omitempty,oneof=ACTIVE CLOSED"`
}

// ProjectMemberResponse is a PM or PC shown with a project.
type ProjectMemberResponse struct {
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// ProjectResponse is the public-facing shape returned by the project API.
type ProjectResponse struct {
	ID          int64                   `json:"id"`
	ProjectCode string                  `json:"project_code"`
	Name        string                  `json:"name"`
	Description *string                 `json:"description"`
	Budget      string                  `json:"budget"`
	StartDate   *string                 `json:"start_date"`
	EndDate     *string                 `json:"end_date"`
	Status      model.ProjectStatus     `json:"status"`
	PM          *ProjectMemberResponse  `json:"pm"`
	PCs         []ProjectMemberResponse `json:"pcs"`
	ClosedAt    *string                 `json:"closed_at,omitempty"`
	CreatedAt   string                  `json:"created_at"`
	UpdatedAt   string                  `json:"updated_at"`
}

// ToProjectResponse converts a project detail to its API response shape.
func ToProjectResponse(detail *model.ProjectDetail) ProjectResponse {
	project := detail.Project

	var startDate, endDate, closedAt *string
	if project.StartDate != nil {
		formatted := project.StartDate.Format(projectDateFormat)
		startDate = &formatted
	}
	if project.EndDate != nil {
		formatted := project.EndDate.Format(projectDateFormat)
		endDate = &formatted
	}
	if project.ClosedAt != nil {
		formatted := project.ClosedAt.Format("2006-01-02T15:04:05Z07:00")
		closedAt = &formatted
	}

	var pm *ProjectMemberResponse
	if detail.PM != nil {
		member := toProjectMemberResponse(*detail.PM)
		pm = &member
	}

	pcs := make([]ProjectMemberResponse, 0, len(detail.PCs))
	for _, pc := range detail.PCs {
		pcs = append(pcs, toProjectMemberResponse(pc))
	}

	return ProjectResponse{
		ID:          project.ID,
		ProjectCode: project.ProjectCode,
		Name:        project.Name,
		Description: project.Description,
		Budget:      project.Budget,
		StartDate:   startDate,
		EndDate:     endDate,
		Status:      project.Status,
		PM:          pm,
		PCs:         pcs,
		ClosedAt:    closedAt,
		CreatedAt:   project.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   project.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toProjectMemberResponse(member model.ProjectMember) ProjectMemberResponse {
	return ProjectMemberResponse{
		UserID:    member.UserID,
		FirstName: member.FirstName,
		LastName:  member.LastName,
		Email:     member.Email,
	}
}
