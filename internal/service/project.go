package service

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
)

const (
	roleAdmin      = "ADMIN"
	rolePM         = "PM"
	rolePC         = "PC"
	roleAccounting = "ACCOUNTING"
)

// budgetPattern accepts up to 13 integer digits and 2 decimals to fit NUMERIC(15,2).
var budgetPattern = regexp.MustCompile(`^\d{1,13}(\.\d{1,2})?$`)

type projectRepository interface {
	WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	Create(ctx context.Context, tx *gorm.DB, project *model.Project) error
	ReplaceCoordinators(ctx context.Context, tx *gorm.DB, projectID int64, userIDs []int64, actorID int64) error
	GetByID(ctx context.Context, id int64) (*model.Project, error)
	GetByIDForUpdate(ctx context.Context, tx *gorm.DB, id int64) (*model.Project, error)
	List(ctx context.Context, filter model.ProjectFilter) ([]*model.Project, int64, error)
	IsCoordinator(ctx context.Context, projectID int64, userID int64) (bool, error)
	Update(ctx context.Context, tx *gorm.DB, id int64, updates map[string]any) error
	FindAssignableUsers(ctx context.Context, userIDs []int64) ([]model.AssignableUser, error)
	ListManagers(ctx context.Context, projectIDs []int64) ([]model.ProjectMember, error)
	ListCoordinators(ctx context.Context, projectIDs []int64) ([]model.ProjectMember, error)
}

// ProjectService contains project business rules.
type ProjectService struct {
	repo projectRepository
}

// NewProjectService creates a ProjectService backed by repo.
func NewProjectService(repo projectRepository) *ProjectService {
	return &ProjectService{repo: repo}
}

// Create creates a project. A PM who creates a project becomes its PM, and a
// PC who creates a project is added to its PCs automatically.
func (s *ProjectService) Create(
	ctx context.Context,
	userID int64,
	role string,
	req *dto.CreateProjectRequest,
) (*model.ProjectDetail, error) {
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}

	code := strings.ToUpper(strings.TrimSpace(req.ProjectCode))
	name := strings.TrimSpace(req.Name)
	if code == "" || name == "" {
		return nil, dto.ErrBadRequest
	}

	budget, err := normalizeBudget(req.Budget)
	if err != nil {
		return nil, err
	}

	startDate, err := parseProjectDate(req.StartDate)
	if err != nil {
		return nil, err
	}
	endDate, err := parseProjectDate(req.EndDate)
	if err != nil {
		return nil, err
	}
	if startDate != nil && endDate != nil && endDate.Before(*startDate) {
		return nil, dto.ErrBadRequest
	}

	var pmUserID int64
	pcUserIDs := append([]int64{}, req.PCUserIDs...)

	switch strings.ToUpper(role) {
	case roleAdmin:
		if req.PMUserID == nil {
			return nil, dto.ErrBadRequest
		}
		pmUserID = *req.PMUserID
	case rolePM:
		pmUserID = userID
	case rolePC:
		if req.PMUserID == nil {
			return nil, dto.ErrBadRequest
		}
		pmUserID = *req.PMUserID
		pcUserIDs = append(pcUserIDs, userID)
	default:
		return nil, dto.ErrForbidden
	}
	pcUserIDs = uniqueIDs(pcUserIDs)

	if err := s.checkMembers(ctx, &pmUserID, pcUserIDs); err != nil {
		return nil, err
	}

	project := &model.Project{
		ProjectCode: code,
		Name:        name,
		Description: trimOptional(req.Description),
		Budget:      budget,
		StartDate:   startDate,
		EndDate:     endDate,
		Status:      model.ProjectStatusActive,
		PMUserID:    pmUserID,
		CreatedBy:   &userID,
		UpdatedBy:   &userID,
	}

	err = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.Create(ctx, tx, project); err != nil {
			return err
		}
		return s.repo.ReplaceCoordinators(ctx, tx, project.ID, pcUserIDs, userID)
	})
	if err != nil {
		if errors.Is(err, dto.ErrProjectCodeExists) {
			return nil, dto.ErrProjectCodeExists
		}
		return nil, dto.ErrInternalServer
	}

	return s.loadDetail(ctx, project.ID)
}

// List returns projects visible to the user. Admin and Accounting see every
// project; PM and PC only see projects they belong to.
func (s *ProjectService) List(
	ctx context.Context,
	userID int64,
	role string,
	filter model.ProjectFilter,
) ([]*model.ProjectDetail, int64, error) {
	switch strings.ToUpper(role) {
	case roleAdmin, roleAccounting:
		filter.MemberUserID = nil
	case rolePM, rolePC:
		filter.MemberUserID = &userID
	default:
		return nil, 0, dto.ErrForbidden
	}

	projects, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, dto.ErrInternalServer
	}

	details, err := s.buildDetails(ctx, projects)
	if err != nil {
		return nil, 0, dto.ErrInternalServer
	}

	return details, total, nil
}

func (s *ProjectService) GetByID(ctx context.Context, userID int64, role string, id int64) (*model.ProjectDetail, error) {
	project, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, dto.ErrProjectNotFound) {
			return nil, dto.ErrProjectNotFound
		}
		return nil, dto.ErrInternalServer
	}

	canView, err := s.canView(ctx, project, userID, role)
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	if !canView {
		return nil, dto.ErrForbidden
	}

	details, err := s.buildDetails(ctx, []*model.Project{project})
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	return details[0], nil
}

// Update partially updates a project.
//   - Admin and the project's PM can change every field.
//   - A PC of the project can only change the name and description.
//   - Only the project's PM can close it, and a closed project cannot change.
func (s *ProjectService) Update(
	ctx context.Context,
	userID int64,
	role string,
	id int64,
	req *dto.UpdateProjectRequest,
) (*model.ProjectDetail, error) {
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}
	role = strings.ToUpper(role)

	err := s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		project, err := s.repo.GetByIDForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if project.Status == model.ProjectStatusClosed {
			return dto.ErrProjectClosed
		}

		isAdmin := role == roleAdmin
		isProjectPM := role == rolePM && project.PMUserID == userID
		isProjectPC := false
		if role == rolePC {
			isProjectPC, err = s.repo.IsCoordinator(ctx, project.ID, userID)
			if err != nil {
				return err
			}
		}
		if !isAdmin && !isProjectPM && !isProjectPC {
			return dto.ErrForbidden
		}

		if isProjectPC && (req.Budget != nil || req.StartDate != nil || req.EndDate != nil ||
			req.PMUserID != nil || req.PCUserIDs != nil || req.Status != nil) {
			return dto.ErrForbidden
		}
		if req.Status != nil && !isProjectPM {
			return dto.ErrForbidden
		}

		updates := make(map[string]any)

		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				return dto.ErrBadRequest
			}
			updates["name"] = name
		}
		if req.Description != nil {
			updates["description"] = trimOptional(req.Description)
		}
		if req.Budget != nil {
			budget, err := normalizeBudget(*req.Budget)
			if err != nil {
				return err
			}
			updates["budget"] = budget
		}

		startDate, endDate := project.StartDate, project.EndDate
		if req.StartDate != nil {
			if startDate, err = parseProjectDate(req.StartDate); err != nil {
				return err
			}
			updates["start_date"] = startDate
		}
		if req.EndDate != nil {
			if endDate, err = parseProjectDate(req.EndDate); err != nil {
				return err
			}
			updates["end_date"] = endDate
		}
		if startDate != nil && endDate != nil && endDate.Before(*startDate) {
			return dto.ErrBadRequest
		}

		if req.PMUserID != nil {
			if err := s.checkMembers(ctx, req.PMUserID, nil); err != nil {
				return err
			}
			updates["pm_user_id"] = *req.PMUserID
		}

		var pcUserIDs []int64
		if req.PCUserIDs != nil {
			pcUserIDs = uniqueIDs(*req.PCUserIDs)
			if err := s.checkMembers(ctx, nil, pcUserIDs); err != nil {
				return err
			}
		}

		if req.Status != nil && *req.Status == string(model.ProjectStatusClosed) {
			// TODO(CHA-31): reject closing while the project still has a fund that is not CLOSED.
			updates["status"] = model.ProjectStatusClosed
			updates["closed_at"] = time.Now()
			updates["closed_by"] = userID
		}

		if len(updates) == 0 && req.PCUserIDs == nil {
			return dto.ErrBadRequest
		}

		updates["updated_by"] = userID
		updates["updated_at"] = time.Now()
		if err := s.repo.Update(ctx, tx, project.ID, updates); err != nil {
			return err
		}

		if req.PCUserIDs != nil {
			return s.repo.ReplaceCoordinators(ctx, tx, project.ID, pcUserIDs, userID)
		}
		return nil
	})
	if err != nil {
		if _, known := dto.ErrorStatusMap[err]; known {
			return nil, err
		}
		return nil, dto.ErrInternalServer
	}

	return s.loadDetail(ctx, id)
}

func (s *ProjectService) canView(ctx context.Context, project *model.Project, userID int64, role string) (bool, error) {
	switch strings.ToUpper(role) {
	case roleAdmin, roleAccounting:
		return true, nil
	case rolePM, rolePC:
		if project.PMUserID == userID {
			return true, nil
		}
		return s.repo.IsCoordinator(ctx, project.ID, userID)
	default:
		return false, nil
	}
}

// checkMembers makes sure the PM has the PM role, every PC has the PC role,
// and all of them have an ACTIVE account.
func (s *ProjectService) checkMembers(ctx context.Context, pmUserID *int64, pcUserIDs []int64) error {
	ids := append([]int64{}, pcUserIDs...)
	if pmUserID != nil {
		ids = append(ids, *pmUserID)
	}

	users, err := s.repo.FindAssignableUsers(ctx, ids)
	if err != nil {
		return dto.ErrInternalServer
	}

	usersByID := make(map[int64]model.AssignableUser, len(users))
	for _, user := range users {
		usersByID[user.UserID] = user
	}

	if pmUserID != nil {
		user, ok := usersByID[*pmUserID]
		if !ok || user.RoleCode != rolePM || user.Status != model.UserStatusActive {
			return dto.ErrInvalidProjectManager
		}
	}
	for _, id := range pcUserIDs {
		user, ok := usersByID[id]
		if !ok || user.RoleCode != rolePC || user.Status != model.UserStatusActive {
			return dto.ErrInvalidProjectCoordinator
		}
	}

	return nil
}

func (s *ProjectService) loadDetail(ctx context.Context, id int64) (*model.ProjectDetail, error) {
	project, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	details, err := s.buildDetails(ctx, []*model.Project{project})
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	return details[0], nil
}

// buildDetails loads the PM and PCs of every project with two queries.
func (s *ProjectService) buildDetails(ctx context.Context, projects []*model.Project) ([]*model.ProjectDetail, error) {
	ids := make([]int64, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}

	managers, err := s.repo.ListManagers(ctx, ids)
	if err != nil {
		return nil, err
	}
	coordinators, err := s.repo.ListCoordinators(ctx, ids)
	if err != nil {
		return nil, err
	}

	managerByProject := make(map[int64]model.ProjectMember, len(managers))
	for _, manager := range managers {
		managerByProject[manager.ProjectID] = manager
	}
	coordinatorsByProject := make(map[int64][]model.ProjectMember)
	for _, coordinator := range coordinators {
		coordinatorsByProject[coordinator.ProjectID] = append(coordinatorsByProject[coordinator.ProjectID], coordinator)
	}

	details := make([]*model.ProjectDetail, 0, len(projects))
	for _, project := range projects {
		detail := &model.ProjectDetail{
			Project: project,
			PCs:     coordinatorsByProject[project.ID],
		}
		if manager, ok := managerByProject[project.ID]; ok {
			detail.PM = &manager
		}
		details = append(details, detail)
	}
	return details, nil
}

func normalizeBudget(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !budgetPattern.MatchString(value) {
		return "", dto.ErrBadRequest
	}
	amount, err := strconv.ParseFloat(value, 64)
	if err != nil || amount <= 0 {
		return "", dto.ErrBadRequest
	}
	return value, nil
}

func parseProjectDate(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", *value)
	if err != nil {
		return nil, dto.ErrBadRequest
	}
	return &parsed, nil
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
