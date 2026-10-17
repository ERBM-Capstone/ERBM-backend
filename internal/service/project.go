package service

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/pkg/pagination"
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
	Create(ctx context.Context, project *model.Project) error
	GetByID(ctx context.Context, id int64) (*model.Project, error)
	GetByIDForUpdate(ctx context.Context, tx *gorm.DB, id int64) (*model.Project, error)
	List(ctx context.Context, filter model.ProjectFilter) ([]*model.Project, int64, error)
	Update(ctx context.Context, tx *gorm.DB, id int64, updates map[string]any) error
	FindAssignableUsers(ctx context.Context, tx *gorm.DB, userIDs []int64) ([]model.AssignableUser, error)
	HasUnclosedAdvances(ctx context.Context, tx *gorm.DB, projectID int64) (bool, error)
	GetMembersByUserIDs(ctx context.Context, userIDs []int64) ([]model.ProjectUserSummary, error)
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
// PC who creates a project becomes its sole PC automatically.
func (s *ProjectService) Create(
	ctx context.Context,
	userID int64,
	role string,
	req *dto.CreateProjectRequest,
) (*model.ProjectDetail, error) {
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}
	if req == nil {
		return nil, dto.ErrBadRequest
	}

	project, err := prepareProjectForCreate(userID, role, req)
	if err != nil {
		return nil, err
	}
	if err := s.checkMembers(ctx, nil, &project.PMUserID, project.PCUserID); err != nil {
		return nil, err
	}

	err = s.repo.Create(ctx, project)
	if err != nil {
		if errors.Is(err, dto.ErrProjectCodeExists) {
			return nil, dto.ErrProjectCodeExists
		}
		return nil, dto.ErrInternalServer
	}

	return s.loadDetail(ctx, project.ID)
}

// prepareProjectForCreate validates input and builds the initial Project.
func prepareProjectForCreate(userID int64, role string, req *dto.CreateProjectRequest) (*model.Project, error) {
	code := strings.ToUpper(strings.TrimSpace(req.ProjectCode))
	if code == "" {
		return nil, dto.ErrBadRequest
	}
	name, err := normalizeProjectName(req.Name)
	if err != nil {
		return nil, err
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
	if err := validateProjectDateRange(startDate, endDate); err != nil {
		return nil, err
	}

	pmUserID, pcUserID, err := resolveProjectCreateMembers(userID, role, req.PMUserID, req.PCUserID)
	if err != nil {
		return nil, err
	}

	return &model.Project{
		ProjectCode: code,
		Name:        name,
		Description: req.Description,
		Budget:      budget,
		StartDate:   startDate,
		EndDate:     endDate,
		Status:      model.ProjectStatusActive,
		PMUserID:    pmUserID,
		PCUserID:    pcUserID,
		CreatedBy:   &userID,
		UpdatedBy:   &userID,
	}, nil
}

// resolveProjectCreateMembers applies the creator's PM/PC assignment rules.
func resolveProjectCreateMembers(userID int64, role string, pmUserID, pcUserID *int64) (int64, *int64, error) {
	role = strings.ToUpper(role)
	switch role {
	case rolePM:
		return userID, pcUserID, nil
	case roleAdmin, rolePC:
		if pmUserID == nil {
			return 0, nil, dto.ErrBadRequest
		}
		if role == rolePC {
			if pcUserID != nil && *pcUserID != userID {
				return 0, nil, dto.ErrBadRequest
			}
			pcUserID = &userID
		}
		return *pmUserID, pcUserID, nil
	default:
		return 0, nil, dto.ErrForbidden
	}
}

// List returns projects visible to the user. Admin and Accounting see every
// project; PM and PC only see projects they belong to.
func (s *ProjectService) List(
	ctx context.Context,
	userID int64,
	role string,
	filter model.ProjectFilter,
) ([]*model.ProjectDetail, int64, error) {
	if userID <= 0 {
		return nil, 0, dto.ErrUnauthorized
	}
	switch strings.ToUpper(role) {
	case roleAdmin, roleAccounting:
		filter.MemberUserID = nil
	case rolePM, rolePC:
		filter.MemberUserID = &userID
	default:
		return nil, 0, dto.ErrForbidden
	}
	filter.Page, filter.PageSize = pagination.WithDefaults(filter.Page, filter.PageSize)
	if !pagination.IsValid(filter.Page, filter.PageSize) ||
		(filter.Status != "" && filter.Status != string(model.ProjectStatusActive) && filter.Status != string(model.ProjectStatusClosed)) ||
		utf8.RuneCountInString(filter.Query) > 255 {
		return nil, 0, dto.ErrBadRequest
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
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}
	project, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, dto.ErrProjectNotFound) {
			return nil, dto.ErrProjectNotFound
		}
		return nil, dto.ErrInternalServer
	}

	if !canViewProject(project, userID, role) {
		return nil, dto.ErrForbidden
	}

	return s.buildDetail(ctx, project)
}

// Update partially updates a project.
//   - Admin and the project's PM can edit project fields and assignments.
//   - A PC of the project can only change the name and description.
//   - Only the project's PM can close it, after all Advances are CLOSED.
//   - A closed project cannot change.
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
	if req == nil {
		return nil, dto.ErrBadRequest
	}

	err := s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		project, err := s.repo.GetByIDForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := authorizeProjectUpdate(project, userID, role, req); err != nil {
			return err
		}
		updates, err := prepareProjectUpdates(project, userID, req)
		if err != nil {
			return err
		}
		if err := s.checkMembers(ctx, tx, req.PMUserID, req.PCUserID); err != nil {
			return err
		}
		if req.Status != nil {
			// Advance creation takes this same Project lock, so no new DRAFT
			// can slip between this check and committing the CLOSED status.
			unclosed, err := s.repo.HasUnclosedAdvances(ctx, tx, project.ID)
			if err != nil {
				return err
			}
			if unclosed {
				return dto.ErrProjectHasUnclosedAdvances
			}
		}
		return s.repo.Update(ctx, tx, project.ID, updates)
	})
	if err != nil {
		if _, known := dto.ErrorStatusMap[err]; known {
			return nil, err
		}
		return nil, dto.ErrInternalServer
	}

	return s.loadDetail(ctx, id)
}

func canViewProject(project *model.Project, userID int64, role string) bool {
	switch strings.ToUpper(role) {
	case roleAdmin, roleAccounting:
		return true
	case rolePM, rolePC:
		return project.PMUserID == userID || (project.PCUserID != nil && *project.PCUserID == userID)
	default:
		return false
	}
}

func authorizeProjectUpdate(project *model.Project, userID int64, role string, req *dto.UpdateProjectRequest) error {
	role = strings.ToUpper(role)
	isProjectPM := role == rolePM && project.PMUserID == userID
	isProjectPC := role == rolePC && project.PCUserID != nil && *project.PCUserID == userID
	if role != roleAdmin && !isProjectPM && !isProjectPC {
		return dto.ErrForbidden
	}
	if isProjectPC && (req.Budget != nil || req.StartDate != nil || req.EndDate != nil ||
		req.PMUserID != nil || req.HasPCUserID() || req.Status != nil) {
		return dto.ErrForbidden
	}
	if req.Status != nil && !isProjectPM {
		return dto.ErrForbidden
	}
	if project.Status == model.ProjectStatusClosed {
		return dto.ErrProjectClosed
	}
	return nil
}

// prepareProjectUpdates builds the entire patch before any field is written.
func prepareProjectUpdates(project *model.Project, userID int64, req *dto.UpdateProjectRequest) (map[string]any, error) {
	updates := make(map[string]any)
	if req.Name != nil {
		name, err := normalizeProjectName(*req.Name)
		if err != nil {
			return nil, err
		}
		updates["name"] = name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Budget != nil {
		budget, err := normalizeBudget(*req.Budget)
		if err != nil {
			return nil, err
		}
		updates["budget"] = budget
	}
	startDate, endDate := project.StartDate, project.EndDate
	if req.StartDate != nil {
		date, err := parseProjectDate(req.StartDate)
		if err != nil {
			return nil, err
		}
		startDate = date
		updates["start_date"] = date
	}
	if req.EndDate != nil {
		date, err := parseProjectDate(req.EndDate)
		if err != nil {
			return nil, err
		}
		endDate = date
		updates["end_date"] = date
	}
	if err := validateProjectDateRange(startDate, endDate); err != nil {
		return nil, err
	}
	if req.PMUserID != nil {
		updates["pm_user_id"] = *req.PMUserID
	}
	if req.HasPCUserID() {
		updates["pc_user_id"] = nil
		if req.PCUserID != nil {
			updates["pc_user_id"] = *req.PCUserID
		}
	}
	now := time.Now()
	if req.Status != nil {
		if *req.Status != string(model.ProjectStatusClosed) {
			return nil, dto.ErrBadRequest
		}
		updates["status"] = model.ProjectStatusClosed
		updates["closed_at"] = now
		updates["closed_by"] = userID
	}
	if len(updates) == 0 {
		return nil, dto.ErrBadRequest
	}
	updates["updated_by"] = userID
	updates["updated_at"] = now
	return updates, nil
}

// checkMembers verifies that the assigned PM/PC have their required roles and ACTIVE accounts.
func (s *ProjectService) checkMembers(ctx context.Context, tx *gorm.DB, pmUserID, pcUserID *int64) error {
	var ids []int64
	if pcUserID != nil {
		ids = append(ids, *pcUserID)
	}
	if pmUserID != nil {
		ids = append(ids, *pmUserID)
	}
	if len(ids) == 0 {
		return nil
	}

	users, err := s.repo.FindAssignableUsers(ctx, tx, ids)
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
	if pcUserID != nil {
		user, ok := usersByID[*pcUserID]
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
	return s.buildDetail(ctx, project)
}

// buildDetail assembles one already loaded Project without reading it again.
func (s *ProjectService) buildDetail(ctx context.Context, project *model.Project) (*model.ProjectDetail, error) {
	details, err := s.buildDetails(ctx, []*model.Project{project})
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	return details[0], nil
}

// buildDetails loads PM/PC profiles in one query using the User IDs already
// stored in the loaded Projects. Later reassignments cannot change these IDs.
func (s *ProjectService) buildDetails(ctx context.Context, projects []*model.Project) ([]*model.ProjectDetail, error) {
	if len(projects) == 0 {
		return []*model.ProjectDetail{}, nil
	}
	assignedUserIDs := make(map[int64]struct{}, 2*len(projects))
	for _, project := range projects {
		assignedUserIDs[project.PMUserID] = struct{}{}
		if project.PCUserID != nil {
			assignedUserIDs[*project.PCUserID] = struct{}{}
		}
	}
	userIDs := make([]int64, 0, len(assignedUserIDs))
	for userID := range assignedUserIDs {
		userIDs = append(userIDs, userID)
	}

	members, err := s.repo.GetMembersByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	memberByUserID := make(map[int64]model.ProjectUserSummary, len(members))
	for _, member := range members {
		memberByUserID[member.UserID] = member
	}

	details := make([]*model.ProjectDetail, 0, len(projects))
	for _, project := range projects {
		detail := &model.ProjectDetail{
			Project: project,
		}
		if manager, ok := memberByUserID[project.PMUserID]; ok {
			detail.PM = &manager
		}
		if project.PCUserID != nil {
			if coordinator, ok := memberByUserID[*project.PCUserID]; ok {
				detail.PC = &coordinator
			}
		}
		details = append(details, detail)
	}
	return details, nil
}

func normalizeProjectName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", dto.ErrBadRequest
	}
	return name, nil
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

func validateProjectDateRange(startDate, endDate *time.Time) error {
	if startDate != nil && endDate != nil && endDate.Before(*startDate) {
		return dto.ErrBadRequest
	}
	return nil
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
