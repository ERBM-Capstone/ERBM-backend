package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
)

func (s *AdvanceService) Approve(ctx context.Context, userID int64, role string, id int64) (*model.AdvanceListItem, error) {
	return s.review(ctx, userID, role, id, model.AdvanceActionApprove, "")
}

func (s *AdvanceService) Return(ctx context.Context, userID int64, role string, id int64, req *dto.ReturnAdvanceRequest) (*model.AdvanceListItem, error) {
	var reason string
	if req != nil {
		reason = req.Reason
	}
	return s.review(ctx, userID, role, id, model.AdvanceActionReturn, reason)
}

func (s *AdvanceService) review(ctx context.Context, userID int64, role string, id int64, action model.AdvanceAction, reason string) (*model.AdvanceListItem, error) {
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}
	if strings.ToUpper(strings.TrimSpace(role)) != rolePM {
		return nil, dto.ErrForbidden
	}
	if id <= 0 {
		return nil, dto.ErrBadRequest
	}
	resultingStatus := model.AdvanceStatusApproved
	switch action {
	case model.AdvanceActionApprove:
	case model.AdvanceActionReturn:
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return nil, dto.ErrBadRequest
		}
		resultingStatus = model.AdvanceStatusReturnedForRevision
	default:
		return nil, dto.ErrBadRequest
	}
	// Project IDs are immutable. Lock the Project before the Advance, as in
	// submission, revision and Project closing, then recheck current authority.
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, advanceDetailError(err)
	}
	var result *model.AdvanceListItem
	err = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		project, err := s.repo.GetProjectForUpdate(ctx, tx, item.ProjectID)
		if err != nil {
			return err
		}
		advance, err := s.repo.GetByIDForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if project.PMUserID != userID {
			return dto.ErrForbidden
		}
		if project.Status == model.ProjectStatusClosed {
			return dto.ErrProjectClosed
		}
		if advance.Status != model.AdvanceStatusPendingPMApproval {
			return dto.ErrAdvanceNotReviewable
		}
		previousStatus := advance.Status
		before, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		advance.Status = resultingStatus
		advance.UpdatedAt, advance.UpdatedBy = now, userID
		if action == model.AdvanceActionApprove {
			advance.ApprovedAt, advance.ApprovedBy = &now, &userID
		}
		if err := s.repo.Review(ctx, tx, advance); err != nil {
			return err
		}
		after, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		beforeData := string(before)
		event := &model.AdvanceWorkflowEvent{
			AdvanceID: id, ActorUserID: userID, Action: action,
			PreviousStatus: &previousStatus, ResultingStatus: resultingStatus,
			BeforeData: &beforeData, AfterData: string(after), OccurredAt: now,
		}
		if action == model.AdvanceActionReturn {
			event.Reason = &reason
		}
		if err := s.repo.RecordEvent(ctx, tx, event); err != nil {
			return err
		}
		result, err = s.repo.GetListItem(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, advanceDetailError(err)
	}
	return result, nil
}
