package dto

import (
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
)

type AdvanceHistoryActorResponse struct {
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type AdvanceHistoryResponse struct {
	ID              int64                       `json:"id"`
	Action          model.AdvanceAction         `json:"action" example:"RETURN"`
	Actor           AdvanceHistoryActorResponse `json:"actor"`
	PreviousStatus  *model.AdvanceStatus        `json:"previous_status" extensions:"x-nullable"`
	ResultingStatus model.AdvanceStatus         `json:"resulting_status"`
	Reason          *string                     `json:"reason" extensions:"x-nullable"`
	Revision        *int64                      `json:"revision" example:"1" extensions:"x-nullable"`
	Amount          *string                     `json:"amount" example:"25000.00" extensions:"x-nullable"`
	Purpose         *string                     `json:"purpose" extensions:"x-nullable"`
	OccurredAt      string                      `json:"occurred_at"`
}

func ToAdvanceHistoryResponse(item model.AdvanceHistoryItem) AdvanceHistoryResponse {
	return AdvanceHistoryResponse{
		ID: item.ID, Action: item.Action,
		Actor: AdvanceHistoryActorResponse{
			UserID: item.ActorUserID, FirstName: item.ActorFirstName, LastName: item.ActorLastName,
		},
		PreviousStatus: item.PreviousStatus, ResultingStatus: item.ResultingStatus,
		Reason: item.Reason, Revision: item.Revision, Amount: item.Amount, Purpose: item.Purpose,
		OccurredAt: item.OccurredAt.Format(time.RFC3339Nano),
	}
}
