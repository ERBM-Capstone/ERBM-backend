package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdvanceRepository struct{ db *gorm.DB }

func NewAdvanceRepository(db *gorm.DB) *AdvanceRepository { return &AdvanceRepository{db: db} }

func (r *AdvanceRepository) GetByID(ctx context.Context, id int64) (*model.AdvanceListItem, error) {
	item, err := r.GetListItem(ctx, r.db, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrAdvanceNotFound
	}
	return item, err
}

func (r *AdvanceRepository) GetByIDForUpdate(ctx context.Context, tx *gorm.DB, id int64) (*model.Advance, error) {
	var advance model.Advance
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&advance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrAdvanceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &advance, nil
}

// Write only editable data and service-owned workflow/audit fields. In
// particular, codes, Project, creator and submission metadata stay unchanged.
func (r *AdvanceRepository) Update(ctx context.Context, tx *gorm.DB, advance *model.Advance) error {
	result := tx.WithContext(ctx).Model(&model.Advance{}).Where("id = ?", advance.ID).Updates(map[string]any{
		"purpose": advance.Purpose, "amount": advance.Amount, "status": advance.Status,
		"updated_by": advance.UpdatedBy, "updated_at": advance.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return dto.ErrAdvanceNotFound
	}
	return nil
}

func (r *AdvanceRepository) WithTx(ctx context.Context, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// Submit writes workflow metadata only; the locked request contents stay intact.
func (r *AdvanceRepository) Submit(ctx context.Context, tx *gorm.DB, advance *model.Advance) error {
	result := tx.WithContext(ctx).Model(&model.Advance{}).Where("id = ?", advance.ID).Updates(map[string]any{
		"status": advance.Status, "submitted_at": advance.SubmittedAt, "submitted_by": advance.SubmittedBy,
		"updated_at": advance.UpdatedAt, "updated_by": advance.UpdatedBy,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return dto.ErrAdvanceNotFound
	}
	return nil
}

func (r *AdvanceRepository) GetProjectForUpdate(ctx context.Context, tx *gorm.DB, projectID int64) (*model.Project, error) {
	var project model.Project
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", projectID).Take(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *AdvanceRepository) IsCoordinator(ctx context.Context, tx *gorm.DB, projectID, userID int64) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).Table("projects").Where("id = ? AND pc_user_id = ?", projectID, userID).Count(&count).Error
	return count > 0, err
}

// The row-level UPSERT lock makes allocation safe across concurrent requests.
// This operation must share the creation transaction so failed writes roll back.
func (r *AdvanceRepository) NextAdvanceNumber(ctx context.Context, tx *gorm.DB, projectID int64) (int64, error) {
	var number int64
	err := tx.WithContext(ctx).Raw(`INSERT INTO project_advance_counters (project_id,last_number) VALUES (?,1)
		ON CONFLICT (project_id) DO UPDATE SET last_number = project_advance_counters.last_number + 1
		RETURNING last_number`, projectID).Scan(&number).Error
	return number, err
}

func (r *AdvanceRepository) Create(ctx context.Context, tx *gorm.DB, advance *model.Advance) error {
	return tx.WithContext(ctx).Create(advance).Error
}

func (r *AdvanceRepository) RecordEvent(ctx context.Context, tx *gorm.DB, event *model.AdvanceWorkflowEvent) error {
	return tx.WithContext(ctx).Create(event).Error
}

const advanceItemColumns = `a.id, a.advance_code, a.project_id, p.project_code, p.name AS project_name, p.pm_user_id, p.pc_user_id,
	a.purpose, a.amount::text AS amount, a.status, a.created_by,
	requester_employee.first_name AS requester_first_name, requester_employee.last_name AS requester_last_name,
	a.submitted_at, a.created_at, a.updated_at`

func advanceItemQuery(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.WithContext(ctx).Table("advances AS a").Joins("JOIN projects AS p ON p.id = a.project_id").
		Joins("JOIN users AS requester ON requester.id = a.created_by").
		Joins("JOIN employees AS requester_employee ON requester_employee.id = requester.employee_id")
}

func (r *AdvanceRepository) GetListItem(ctx context.Context, tx *gorm.DB, id int64) (*model.AdvanceListItem, error) {
	var item model.AdvanceListItem
	if err := advanceItemQuery(ctx, tx).Select(advanceItemColumns).Where("a.id = ?", id).Take(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AdvanceRepository) GetProjectAccess(ctx context.Context, projectID, userID int64) (*model.AdvanceProjectAccess, error) {
	var access model.AdvanceProjectAccess
	err := r.db.WithContext(ctx).Table("projects AS p").
		Select(`p.id AS project_id, p.pm_user_id,
			COALESCE(p.pc_user_id = ?, FALSE) AS is_coordinator`, userID).
		Where("p.id = ?", projectID).Take(&access).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return &access, nil
}

func (r *AdvanceRepository) GetSubmissionHistory(ctx context.Context, id int64) (*model.AdvanceSubmissionHistory, error) {
	type historyRow struct {
		model.AdvanceListItem `gorm:"embedded"`
		EventID               *int64
		ActorUserID           int64
		AfterData             string
		OccurredAt            *time.Time
	}
	var rows []historyRow
	// LEFT JOIN distinguishes an unsubmitted Advance from a missing Advance.
	// Status, membership and submission snapshots share this one SQL statement.
	err := advanceItemQuery(ctx, r.db).
		Joins("LEFT JOIN advance_workflow_events AS e ON e.advance_id = a.id AND e.action = ?", model.AdvanceActionSubmit).
		Select(advanceItemColumns+", e.id AS event_id, e.actor_user_id, e.after_data::text AS after_data, e.occurred_at").
		Where("a.id = ?", id).Order("e.id ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, dto.ErrAdvanceNotFound
	}
	history := &model.AdvanceSubmissionHistory{Advance: rows[0].AdvanceListItem, Submissions: make([]model.AdvanceSubmission, 0, len(rows))}
	for _, row := range rows {
		if row.EventID != nil {
			history.Submissions = append(history.Submissions, model.AdvanceSubmission{
				ActorUserID: row.ActorUserID, AfterData: row.AfterData, OccurredAt: *row.OccurredAt,
			})
		}
	}
	return history, nil
}

func (r *AdvanceRepository) List(ctx context.Context, filter model.AdvanceFilter) ([]model.AdvanceListItem, int64, error) {
	if filter.ActorUserID <= 0 {
		return nil, 0, dto.ErrUnauthorized
	}
	db := advanceItemQuery(ctx, r.db)
	switch filter.ActorRole {
	case "ADMIN", "ACCOUNTING":
	case "PM":
		db = db.Where("p.pm_user_id = ?", filter.ActorUserID)
	case "PC":
		db = db.Where("p.pc_user_id = ?", filter.ActorUserID)
	default:
		return nil, 0, dto.ErrForbidden
	}
	// Apply the same private-DRAFT and Project scope to count and select.
	db = db.Where("(a.status <> ? OR a.created_by = ?)", model.AdvanceStatusDraft, filter.ActorUserID)
	if filter.ProjectID != nil {
		db = db.Where("a.project_id = ?", *filter.ProjectID)
	}
	if filter.Status != "" {
		db = db.Where("a.status = ?", filter.Status)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		db = db.Where("a.purpose ILIKE ?", "%"+q+"%")
	}
	// Clone the filtered statement for each operation so count and select do
	// not share execution state, including SQL retained during dry runs.
	db = db.Session(&gorm.Session{})
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.AdvanceListItem, 0)
	err := db.Select(advanceItemColumns).
		Order("a.created_at DESC, a.id DESC").Limit(filter.PageSize).
		Offset((filter.Page - 1) * filter.PageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
