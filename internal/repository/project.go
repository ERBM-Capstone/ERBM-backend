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

// ProjectRepository handles database operations for projects and their PCs.
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

// Create inserts a project using the caller's transaction.
func (r *ProjectRepository) Create(ctx context.Context, tx *gorm.DB, project *model.Project) error {
	if err := tx.WithContext(ctx).Create(project).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return dto.ErrProjectCodeExists
		}
		return err
	}
	return nil
}

// ReplaceCoordinators removes the current PCs of a project and inserts userIDs.
func (r *ProjectRepository) ReplaceCoordinators(
	ctx context.Context,
	tx *gorm.DB,
	projectID int64,
	userIDs []int64,
	actorID int64,
) error {
	if err := tx.WithContext(ctx).
		Where("project_id = ?", projectID).
		Delete(&model.ProjectCoordinator{}).Error; err != nil {
		return err
	}

	if len(userIDs) == 0 {
		return nil
	}

	coordinators := make([]model.ProjectCoordinator, 0, len(userIDs))
	for _, userID := range userIDs {
		coordinators = append(coordinators, model.ProjectCoordinator{
			ProjectID: projectID,
			UserID:    userID,
			CreatedBy: &actorID,
		})
	}

	return tx.WithContext(ctx).Create(&coordinators).Error
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
			"pm_user_id = ? OR id IN (SELECT project_id FROM project_coordinators WHERE user_id = ?)",
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
		Model(&model.ProjectCoordinator{}).
		Where("project_id = ? AND user_id = ?", projectID, userID).
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
func (r *ProjectRepository) FindAssignableUsers(ctx context.Context, userIDs []int64) ([]model.AssignableUser, error) {
	var users []model.AssignableUser
	if len(userIDs) == 0 {
		return users, nil
	}

	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select("u.id AS user_id, r.code AS role_code, u.status").
		Joins("JOIN roles AS r ON r.id = u.role_id").
		Where("u.id IN ?", userIDs).
		Scan(&users).Error
	return users, err
}

// ListManagers returns the PM of each given project.
func (r *ProjectRepository) ListManagers(ctx context.Context, projectIDs []int64) ([]model.ProjectMember, error) {
	var members []model.ProjectMember
	if len(projectIDs) == 0 {
		return members, nil
	}

	err := r.db.WithContext(ctx).
		Table("projects AS p").
		Select("p.id AS project_id, u.id AS user_id, e.first_name, e.last_name, u.email").
		Joins("JOIN users AS u ON u.id = p.pm_user_id").
		Joins("JOIN employees AS e ON e.id = u.employee_id").
		Where("p.id IN ?", projectIDs).
		Scan(&members).Error
	return members, err
}

// ListCoordinators returns the PCs of each given project.
func (r *ProjectRepository) ListCoordinators(ctx context.Context, projectIDs []int64) ([]model.ProjectMember, error) {
	var members []model.ProjectMember
	if len(projectIDs) == 0 {
		return members, nil
	}

	err := r.db.WithContext(ctx).
		Table("project_coordinators AS pc").
		Select("pc.project_id, u.id AS user_id, e.first_name, e.last_name, u.email").
		Joins("JOIN users AS u ON u.id = pc.user_id").
		Joins("JOIN employees AS e ON e.id = u.employee_id").
		Where("pc.project_id IN ?", projectIDs).
		Order("pc.created_at ASC, u.id ASC").
		Scan(&members).Error
	return members, err
}
