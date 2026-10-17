package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProjectRepository handles database operations for projects and their assigned PC.
type ProjectRepository struct {
	db *gorm.DB
}

// NewProjectRepository creates a ProjectRepository backed by db.
func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// WithTx runs fn in a GORM transaction.
func (r *ProjectRepository) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// Create inserts a project using GORM's default transaction.
func (r *ProjectRepository) Create(ctx context.Context, project *model.Project) error {
	if err := r.db.WithContext(ctx).Create(project).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return dto.ErrProjectCodeExists
		}
		return err
	}
	return nil
}

func (r *ProjectRepository) GetByID(ctx context.Context, id int64) (*model.Project, error) {
	var project model.Project
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, dto.ErrProjectNotFound
		}
		return nil, err
	}
	return &project, nil
}

// GetByIDForUpdate loads a project and locks its row until the transaction ends,
// so two updates (for example edit and close) cannot overwrite each other.
func (r *ProjectRepository) GetByIDForUpdate(ctx context.Context, tx *gorm.DB, id int64) (*model.Project, error) {
	var project model.Project
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		Take(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, dto.ErrProjectNotFound
		}
		return nil, err
	}
	return &project, nil
}

func (r *ProjectRepository) List(ctx context.Context, filter model.ProjectFilter) ([]*model.Project, int64, error) {
	var projects []*model.Project
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Project{})

	if filter.MemberUserID != nil {
		db = db.Where(
			"pm_user_id = ? OR pc_user_id = ?",
			*filter.MemberUserID, *filter.MemberUserID,
		)
	}
	if filter.Status != "" {
		db = db.Where("status = ?", filter.Status)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + q + "%"
		db = db.Where("(project_code ILIKE ? OR name ILIKE ?)", like, like)
	}

	// Count and page selection share filters, but not execution state.
	db = db.Session(&gorm.Session{})
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	if err := db.
		Order("id DESC").
		Limit(filter.PageSize).
		Offset(offset).
		Find(&projects).Error; err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}

func (r *ProjectRepository) IsCoordinator(ctx context.Context, projectID int64, userID int64) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.Project{}).
		Where("id = ? AND pc_user_id = ?", projectID, userID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ProjectRepository) Update(ctx context.Context, tx *gorm.DB, id int64, updates map[string]any) error {
	return tx.WithContext(ctx).
		Model(&model.Project{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// FindAssignableUsers returns the role and account status of the given users.
// Users that do not exist are simply missing from the result.
func (r *ProjectRepository) FindAssignableUsers(ctx context.Context, tx *gorm.DB, userIDs []int64) ([]model.AssignableUser, error) {
	var users []model.AssignableUser
	if len(userIDs) == 0 {
		return users, nil
	}

	if tx == nil {
		tx = r.db
	}
	err := tx.WithContext(ctx).
		Table("users AS u").
		Select("u.id AS user_id, r.code AS role_code, u.status").
		Joins("JOIN roles AS r ON r.id = u.role_id").
		Where("u.id IN ?", userIDs).
		Scan(&users).Error
	return users, err
}

// HasUnclosedAdvances must run while the caller holds the Project row lock.
// Every status other than CLOSED blocks Project closing, including DRAFT.
func (r *ProjectRepository) HasUnclosedAdvances(ctx context.Context, tx *gorm.DB, projectID int64) (bool, error) {
	var advance model.Advance
	err := tx.WithContext(ctx).Select("id").
		Where("project_id = ? AND status <> ?", projectID, model.AdvanceStatusClosed).
		Take(&advance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

// GetMembersByUserIDs loads PM/PC profiles using the assignments already read
// from Projects, without re-reading assignments that may have since changed.
func (r *ProjectRepository) GetMembersByUserIDs(ctx context.Context, userIDs []int64) ([]model.ProjectUserSummary, error) {
	members := make([]model.ProjectUserSummary, 0)
	if len(userIDs) == 0 {
		return members, nil
	}

	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select("u.id AS user_id, e.first_name, e.last_name, u.email").
		Joins("JOIN employees AS e ON e.id = u.employee_id").
		Where("u.id IN ?", userIDs).
		Find(&members).Error
	return members, err
}
