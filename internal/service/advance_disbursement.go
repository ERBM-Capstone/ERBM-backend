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

// Disburse confirms a single full payment of the approved amount. The request
// contains no financial or actor fields; all values come from the locked
// Advance, authenticated Accounting user and server clock.
func (s *AdvanceService) Disburse(ctx context.Context, userID int64, role string, id int64) (*model.AdvanceListItem, error) {
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}
	if strings.ToUpper(strings.TrimSpace(role)) != roleAccounting {
		return nil, dto.ErrForbidden
	}
	if id <= 0 {
		return nil, dto.ErrBadRequest
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, advanceDetailError(err)
	}
	var result *model.AdvanceListItem
	err = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		// The same Project lock serializes competing disbursements and
		// Project closing. Recheck both statuses under Project -> Advance locks.
		project, err := s.repo.GetProjectForUpdate(ctx, tx, item.ProjectID)
		if err != nil {
			return err
		}
		advance, err := s.repo.GetByIDForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if project.Status == model.ProjectStatusClosed {
			return dto.ErrProjectClosed
		}
		if advance.Status != model.AdvanceStatusApproved {
			return dto.ErrAdvanceNotDisbursable
		}
		active, err := s.repo.HasActiveAdvance(ctx, tx, project.ID)
		if err != nil {
			return err
		}
		if active {
			return dto.ErrProjectHasActiveAdvance
		}
		amount, err := normalizeAdvanceAmount(advance.Amount)
		if err != nil {
			return err
		}
		previousStatus := advance.Status
		before, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		payment := &model.AdvanceDisbursement{
			AdvanceID: id, Amount: amount, DisbursedAt: now,
			ConfirmedBy: userID, ConfirmedAt: now,
		}
		if err := s.repo.CreateDisbursement(ctx, tx, payment); err != nil {
			return err
		}
		advance.Status = model.AdvanceStatusActive
		advance.ActivatedAt = &now
		advance.UpdatedBy, advance.UpdatedAt = userID, now
		if err := s.repo.Activate(ctx, tx, advance); err != nil {
			return err
		}
		after, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		beforeData := string(before)
		if err := s.repo.RecordEvent(ctx, tx, &model.AdvanceWorkflowEvent{
			AdvanceID: id, ActorUserID: userID, Action: model.AdvanceActionDisburse,
			PreviousStatus: &previousStatus, ResultingStatus: model.AdvanceStatusActive,
			BeforeData: &beforeData, AfterData: string(after), OccurredAt: now,
		}); err != nil {
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
