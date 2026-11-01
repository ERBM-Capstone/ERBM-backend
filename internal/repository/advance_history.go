package repository

import (
	"context"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
)

func (r *AdvanceRepository) GetHistory(ctx context.Context, id int64) (*model.AdvanceHistory, error) {
	type historyRow struct {
		model.AdvanceListItem `gorm:"embedded"`
		EventID               *int64
		Action                model.AdvanceAction
		ActorUserID           int64
		ActorFirstName        string
		ActorLastName         string
		PreviousStatus        *model.AdvanceStatus
		ResultingStatus       model.AdvanceStatus
		Reason                *string
		SubmittedAmount       *string
		SubmittedPurpose      *string
		OccurredAt            time.Time
	}
	var rows []historyRow
	// One statement keeps authorization and history in the same database
	// snapshot. LEFT JOIN retains Advances with no events and former actors.
	err := advanceItemQuery(ctx, r.db).
		Joins("LEFT JOIN advance_workflow_events AS e ON e.advance_id = a.id").
		Joins("LEFT JOIN users AS actor_user ON actor_user.id = e.actor_user_id").
		Joins("LEFT JOIN employees AS actor_employee ON actor_employee.id = actor_user.employee_id").
		Select(advanceItemColumns+`, e.id AS event_id, e.action, e.actor_user_id,
			actor_employee.first_name AS actor_first_name, actor_employee.last_name AS actor_last_name,
			e.previous_status, e.resulting_status, e.reason, e.occurred_at,
			CASE WHEN e.action = ? THEN e.after_data->>'amount' ELSE NULL END AS submitted_amount,
			CASE WHEN e.action = ? THEN e.after_data->>'purpose' ELSE NULL END AS submitted_purpose`, model.AdvanceActionSubmit, model.AdvanceActionSubmit).
		Where("a.id = ?", id).Order("e.occurred_at DESC, e.id DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, dto.ErrAdvanceNotFound
	}
	history := &model.AdvanceHistory{
		Advance: rows[0].AdvanceListItem,
		Events:  make([]model.AdvanceHistoryItem, 0, len(rows)),
	}
	for _, row := range rows {
		if row.EventID != nil {
			history.Events = append(history.Events, model.AdvanceHistoryItem{
				ID: *row.EventID, Action: row.Action, ActorUserID: row.ActorUserID,
				ActorFirstName: row.ActorFirstName, ActorLastName: row.ActorLastName,
				PreviousStatus: row.PreviousStatus, ResultingStatus: row.ResultingStatus,
				Reason: row.Reason, Amount: row.SubmittedAmount, Purpose: row.SubmittedPurpose,
				OccurredAt: row.OccurredAt,
			})
		}
	}
	return history, nil
}
