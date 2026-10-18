package dto

import (
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
)

type CreateAdvanceRequest struct {
	ProjectID int64  `json:"project_id" binding:"required,min=1"`
	Purpose   string `json:"purpose" binding:"required"`
	Amount    string `json:"amount" binding:"required" example:"15000.00"`
}

// Nil fields retain their current values. At least one field must be supplied.
type UpdateAdvanceRequest struct {
	Purpose *string `json:"purpose" binding:"omitempty,min=1" extensions:"x-nullable"`
	Amount  *string `json:"amount" binding:"omitempty,min=1" example:"15000.00" extensions:"x-nullable"`
}

// AdvanceListQuery contains Advance filters and pagination. The HTTP handler
// applies paging defaults to omitted or zero page/page_size values.
type AdvanceListQuery struct {
	ProjectID *int64              `form:"project_id" binding:"omitempty,min=1"`
	Status    model.AdvanceStatus `form:"status" binding:"omitempty,oneof=DRAFT PENDING_PM_APPROVAL RETURNED_FOR_REVISION APPROVED ACTIVE CLOSED"`
	Q         string              `form:"q" binding:"omitempty,max=255"`
	Page      int                 `form:"page" binding:"omitempty,min=1"`
	PageSize  int                 `form:"page_size" binding:"omitempty,min=1,max=100"`
}

type AdvanceProjectResponse struct {
	ID          int64  `json:"id"`
	ProjectCode string `json:"project_code"`
	Name        string `json:"name"`
}

type AdvanceListItemResponse struct {
	ID          int64                    `json:"id"`
	AdvanceCode string                   `json:"advance_code" example:"ADV-PRJ001-001"`
	Requester   AdvanceRequesterResponse `json:"requester"`
	Project     AdvanceProjectResponse   `json:"project"`
	Purpose     string                   `json:"purpose"`
	Amount      string                   `json:"amount"`
	Status      model.AdvanceStatus      `json:"status"`
	CreatedBy   int64                    `json:"created_by"`
	SubmittedAt *string                  `json:"submitted_at" extensions:"x-nullable"`
	CreatedAt   string                   `json:"created_at"`
	UpdatedAt   string                   `json:"updated_at"`
}

type AdvanceRequesterResponse struct {
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func ToAdvanceListItemResponse(item model.AdvanceListItem) AdvanceListItemResponse {
	var submittedAt *string
	if item.SubmittedAt != nil {
		formatted := item.SubmittedAt.Format(time.RFC3339)
		submittedAt = &formatted
	}
	return AdvanceListItemResponse{
		ID:          item.ID,
		AdvanceCode: item.AdvanceCode,
		Requester:   AdvanceRequesterResponse{UserID: item.CreatedBy, FirstName: item.RequesterFirstName, LastName: item.RequesterLastName},
		Project:     AdvanceProjectResponse{ID: item.ProjectID, ProjectCode: item.ProjectCode, Name: item.ProjectName},
		Purpose:     item.Purpose, Amount: item.Amount, Status: item.Status,
		CreatedBy: item.CreatedBy, SubmittedAt: submittedAt,
		CreatedAt: item.CreatedAt.Format(time.RFC3339), UpdatedAt: item.UpdatedAt.Format(time.RFC3339),
	}
}
