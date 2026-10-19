package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/pkg/pagination"
	"gorm.io/gorm"
)

type advanceRepository interface {
	GetByID(context.Context, int64) (*model.AdvanceListItem, error)
	GetByIDForUpdate(context.Context, *gorm.DB, int64) (*model.Advance, error)
	Update(context.Context, *gorm.DB, *model.Advance) error
	Submit(context.Context, *gorm.DB, *model.Advance) error
	GetSubmissionHistory(context.Context, int64) (*model.AdvanceSubmissionHistory, error)
	List(context.Context, model.AdvanceFilter) ([]model.AdvanceListItem, int64, error)
	GetProjectAccess(context.Context, int64, int64) (*model.AdvanceProjectAccess, error)
	WithTx(context.Context, func(*gorm.DB) error) error
	GetProjectForUpdate(context.Context, *gorm.DB, int64) (*model.Project, error)
	IsCoordinator(context.Context, *gorm.DB, int64, int64) (bool, error)
	NextAdvanceNumber(context.Context, *gorm.DB, int64) (int64, error)
	Create(context.Context, *gorm.DB, *model.Advance) error
	RecordEvent(context.Context, *gorm.DB, *model.AdvanceWorkflowEvent) error
	GetListItem(context.Context, *gorm.DB, int64) (*model.AdvanceListItem, error)
}

type AdvanceService struct{ repo advanceRepository }

func NewAdvanceService(repo advanceRepository) *AdvanceService { return &AdvanceService{repo: repo} }

var advanceAmountPattern = regexp.MustCompile(`^[0-9]{1,13}(\.[0-9]{1,2})?$`)

// normalizeAdvanceAmount validates NUMERIC(15,2) without floating-point parsing
// or silently rounding the caller's amount.
func normalizeAdvanceAmount(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !advanceAmountPattern.MatchString(value) || strings.Trim(value, "0.") == "" {
		return "", dto.ErrBadRequest
	}
	whole, fraction, _ := strings.Cut(value, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	fraction += strings.Repeat("0", 2-len(fraction))
	return whole + "." + fraction, nil
}

func (s *AdvanceService) Create(ctx context.Context, userID int64, role string, req *dto.CreateAdvanceRequest) (*model.AdvanceListItem, error) {

	if strings.ToUpper(strings.TrimSpace(role)) != rolePC {
		return nil, dto.ErrForbidden
	}
	if req == nil || req.ProjectID <= 0 || strings.TrimSpace(req.Purpose) == "" {
		return nil, dto.ErrBadRequest
	}
	amount, err := normalizeAdvanceAmount(req.Amount)
	if err != nil {
		return nil, err
	}
	var result *model.AdvanceListItem
	err = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		// Project updates/closing use this same row lock, so creation cannot
		// race with closing or a change in coordinator assignments.
		project, err := s.repo.GetProjectForUpdate(ctx, tx, req.ProjectID)
		if err != nil {
			return err
		}
		member, err := s.repo.IsCoordinator(ctx, tx, project.ID, userID)
		if err != nil {
			return err
		}
		if !member {
			return dto.ErrForbidden
		}
		if project.Status == model.ProjectStatusClosed {
			return dto.ErrProjectClosed
		}
		number, err := s.repo.NextAdvanceNumber(ctx, tx, project.ID)
		if err != nil {
			return err
		}
		advance := &model.Advance{
			AdvanceCode: fmt.Sprintf("ADV-%s-%03d", project.ProjectCode, number),
			ProjectID:   project.ID, Purpose: strings.TrimSpace(req.Purpose), Amount: amount,
			Status: model.AdvanceStatusDraft, CreatedBy: userID, UpdatedBy: userID,
		}
		// Existing ACTIVE records do not block DRAFT creation. The partial
		// unique index enforces one ACTIVE when activation is implemented.
		if err := s.repo.Create(ctx, tx, advance); err != nil {
			return err
		}
		snapshot, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		if err := s.repo.RecordEvent(ctx, tx, &model.AdvanceWorkflowEvent{
			AdvanceID: advance.ID, ActorUserID: userID, Action: model.AdvanceActionCreate,
			ResultingStatus: model.AdvanceStatusDraft, AfterData: string(snapshot), OccurredAt: advance.CreatedAt,
		}); err != nil {
			return err
		}
		result, err = s.repo.GetListItem(ctx, tx, advance.ID)
		return err
	})
	if err != nil {
		switch {
		case errors.Is(err, dto.ErrProjectNotFound):
			return nil, dto.ErrProjectNotFound
		case errors.Is(err, dto.ErrForbidden):
			return nil, dto.ErrForbidden
		case errors.Is(err, dto.ErrProjectClosed):
			return nil, dto.ErrProjectClosed
		default:
			return nil, dto.ErrInternalServer
		}
	}
	return result, nil
}

func (s *AdvanceService) List(ctx context.Context, userID int64, role string, filter model.AdvanceFilter) ([]model.AdvanceListItem, int64, error) {
	if userID <= 0 {
		return nil, 0, dto.ErrUnauthorized
	}
	role = strings.ToUpper(strings.TrimSpace(role))
	switch role {
	case roleAdmin, roleAccounting, rolePM, rolePC:
	default:
		return nil, 0, dto.ErrForbidden
	}
	if !pagination.IsValid(filter.Page, filter.PageSize) ||
		(filter.ProjectID != nil && *filter.ProjectID <= 0) ||
		(filter.Status != "" && !filter.Status.IsKnown()) || utf8.RuneCountInString(filter.Query) > 255 {
		return nil, 0, dto.ErrBadRequest
	}
	if filter.ProjectID != nil {
		access, err := s.repo.GetProjectAccess(ctx, *filter.ProjectID, userID)
		if errors.Is(err, dto.ErrProjectNotFound) {
			return nil, 0, dto.ErrProjectNotFound
		}
		if err != nil || access == nil {
			return nil, 0, dto.ErrInternalServer
		}
		if (role == rolePM && access.PMUserID != userID) || (role == rolePC && !access.IsCoordinator) {
			return nil, 0, dto.ErrForbidden
		}
	}
	filter.ActorUserID, filter.ActorRole = userID, role
	filter.Query = strings.TrimSpace(filter.Query)
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, dto.ErrInternalServer
	}
	return items, total, nil
}

func (s *AdvanceService) GetByID(ctx context.Context, userID int64, role string, id int64) (*model.AdvanceListItem, error) {
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}
	role = strings.ToUpper(strings.TrimSpace(role))
	switch role {
	case roleAdmin, roleAccounting, rolePM, rolePC:
	default:
		return nil, dto.ErrForbidden
	}
	if id <= 0 {
		return nil, dto.ErrBadRequest
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, advanceDetailError(err)
	}
	if err := authorizeAdvanceRead(item, userID, role); err != nil {
		return nil, err
	}
	return item, nil
}

func authorizeAdvanceRead(item *model.AdvanceListItem, userID int64, role string) error {
	if item.Status == model.AdvanceStatusDraft && item.CreatedBy != userID {
		return dto.ErrForbidden
	}
	// Authorize against the Project columns read in the same statement as
	// status and content, avoiding a mix of old content and new membership.
	if (role == rolePM && item.PMUserID != userID) ||
		(role == rolePC && (item.PCUserID == nil || *item.PCUserID != userID)) {
		return dto.ErrForbidden
	}
	return nil
}

func (s *AdvanceService) Revisions(ctx context.Context, userID int64, role string, id int64) ([]model.AdvanceRevision, error) {

	role = strings.ToUpper(strings.TrimSpace(role))
	switch role {
	case roleAdmin, roleAccounting, rolePM, rolePC:
	default:
		return nil, dto.ErrForbidden
	}
	if id <= 0 {
		return nil, dto.ErrBadRequest
	}
	history, err := s.repo.GetSubmissionHistory(ctx, id)
	if err != nil {
		return nil, advanceDetailError(err)
	}
	if err := authorizeAdvanceRead(&history.Advance, userID, role); err != nil {
		return nil, err
	}
	revisions := make([]model.AdvanceRevision, 0, len(history.Submissions))
	for i, submission := range history.Submissions {
		var snapshot model.Advance
		if err := json.Unmarshal([]byte(submission.AfterData), &snapshot); err != nil {
			return nil, dto.ErrInternalServer
		}
		revisions = append(revisions, model.AdvanceRevision{
			Revision: int64(i + 1), SubmittedBy: submission.ActorUserID,
			SubmittedAt: submission.OccurredAt, Snapshot: snapshot,
		})
	}
	return revisions, nil
}

func (s *AdvanceService) Update(ctx context.Context, userID int64, role string, id int64, req *dto.UpdateAdvanceRequest) (*model.AdvanceListItem, error) {
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}
	if strings.ToUpper(strings.TrimSpace(role)) != rolePC {
		return nil, dto.ErrForbidden
	}
	if id <= 0 || req == nil || (req.Purpose == nil && req.Amount == nil) {
		return nil, dto.ErrBadRequest
	}
	var purpose, amount string
	if req.Purpose != nil {
		purpose = strings.TrimSpace(*req.Purpose)
		if purpose == "" {
			return nil, dto.ErrBadRequest
		}
	}
	if req.Amount != nil {
		var err error
		amount, err = normalizeAdvanceAmount(*req.Amount)
		if err != nil {
			return nil, err
		}
	}
	// Project IDs are immutable. Look up the parent before taking locks in
	// Project -> Advance order, shared with creation and Project updates.
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
		if advance.CreatedBy != userID || project.PCUserID == nil || *project.PCUserID != userID {
			return dto.ErrForbidden
		}
		if project.Status == model.ProjectStatusClosed {
			return dto.ErrProjectClosed
		}
		if advance.Status != model.AdvanceStatusDraft && advance.Status != model.AdvanceStatusReturnedForRevision {
			return dto.ErrAdvanceNotEditable
		}
		previousStatus := advance.Status
		before, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		if req.Purpose != nil {
			advance.Purpose = purpose
		}
		if req.Amount != nil {
			advance.Amount = amount
		}
		advance.Status = model.AdvanceStatusDraft
		advance.UpdatedBy, advance.UpdatedAt = userID, time.Now().UTC()
		if err := s.repo.Update(ctx, tx, advance); err != nil {
			return err
		}
		after, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		beforeData := string(before)
		if err := s.repo.RecordEvent(ctx, tx, &model.AdvanceWorkflowEvent{
			AdvanceID: id, ActorUserID: userID, Action: model.AdvanceActionUpdate,
			PreviousStatus: &previousStatus, ResultingStatus: advance.Status,
			BeforeData: &beforeData, AfterData: string(after), OccurredAt: advance.UpdatedAt,
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

func (s *AdvanceService) Submit(ctx context.Context, userID int64, role string, id int64) (*model.AdvanceListItem, error) {
	if strings.ToUpper(strings.TrimSpace(role)) != rolePC {
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
		// Keep the Project -> Advance lock order used by editing and closing.
		project, err := s.repo.GetProjectForUpdate(ctx, tx, item.ProjectID)
		if err != nil {
			return err
		}
		advance, err := s.repo.GetByIDForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if advance.CreatedBy != userID || project.PCUserID == nil || *project.PCUserID != userID {
			return dto.ErrForbidden
		}
		if project.Status == model.ProjectStatusClosed {
			return dto.ErrProjectClosed
		}
		if advance.Status != model.AdvanceStatusDraft {
			return dto.ErrAdvanceNotSubmittable
		}
		if strings.TrimSpace(advance.Purpose) == "" {
			return dto.ErrBadRequest
		}
		if _, err := normalizeAdvanceAmount(advance.Amount); err != nil {
			return err
		}
		previousStatus := advance.Status
		before, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		advance.Status = model.AdvanceStatusPendingPMApproval
		advance.SubmittedAt, advance.SubmittedBy = &now, &userID
		advance.UpdatedAt, advance.UpdatedBy = now, userID
		if err := s.repo.Submit(ctx, tx, advance); err != nil {
			return err
		}
		after, err := json.Marshal(advance)
		if err != nil {
			return err
		}
		beforeData := string(before)
		if err := s.repo.RecordEvent(ctx, tx, &model.AdvanceWorkflowEvent{
			AdvanceID: id, ActorUserID: userID, Action: model.AdvanceActionSubmit,
			PreviousStatus: &previousStatus, ResultingStatus: advance.Status,
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

func advanceDetailError(err error) error {
	for _, known := range []error{dto.ErrAdvanceNotFound, dto.ErrForbidden, dto.ErrProjectClosed, dto.ErrAdvanceNotEditable, dto.ErrAdvanceNotSubmittable, dto.ErrBadRequest} {
		if errors.Is(err, known) {
			return known
		}
	}
	return dto.ErrInternalServer
}
