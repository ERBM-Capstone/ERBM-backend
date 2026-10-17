package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
)

// fakeProjectRepo keeps projects in memory so service rules can be tested
// without a database.
type fakeProjectRepo struct {
	users     map[int64]model.AssignableUser
	projects  map[int64]*model.Project
	nextID    int64
	createErr error
}

func newFakeProjectRepo() *fakeProjectRepo {
	return &fakeProjectRepo{
		users: map[int64]model.AssignableUser{
			1: {UserID: 1, RoleCode: "ADMIN", Status: model.UserStatusActive},
			2: {UserID: 2, RoleCode: "PM", Status: model.UserStatusActive},
			3: {UserID: 3, RoleCode: "PC", Status: model.UserStatusActive},
			4: {UserID: 4, RoleCode: "PC", Status: model.UserStatusActive},
			5: {UserID: 5, RoleCode: "ACCOUNTING", Status: model.UserStatusActive},
			6: {UserID: 6, RoleCode: "PC", Status: model.UserStatusInvited},
			7: {UserID: 7, RoleCode: "PM", Status: model.UserStatusActive},
		},
		projects: map[int64]*model.Project{},
	}
}

func (r *fakeProjectRepo) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return fn(nil)
}

func (r *fakeProjectRepo) NextProjectNumber(context.Context, *gorm.DB, int) (int64, error) {
	return r.nextID + 1, nil
}

func (r *fakeProjectRepo) Create(ctx context.Context, _ *gorm.DB, project *model.Project) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.nextID++
	project.ID = r.nextID
	r.projects[project.ID] = project
	return nil
}

func (r *fakeProjectRepo) GetByID(ctx context.Context, id int64) (*model.Project, error) {
	project, ok := r.projects[id]
	if !ok {
		return nil, dto.ErrProjectNotFound
	}
	return project, nil
}

func (r *fakeProjectRepo) GetByIDForUpdate(ctx context.Context, tx *gorm.DB, id int64) (*model.Project, error) {
	return r.GetByID(ctx, id)
}

func (r *fakeProjectRepo) List(ctx context.Context, filter model.ProjectFilter) ([]*model.Project, int64, error) {
	var result []*model.Project
	for _, project := range r.projects {
		if filter.MemberUserID != nil {
			isMember, _ := r.IsCoordinator(ctx, project.ID, *filter.MemberUserID)
			if project.PMUserID != *filter.MemberUserID && !isMember {
				continue
			}
		}
		result = append(result, project)
	}
	return result, int64(len(result)), nil
}

func (r *fakeProjectRepo) IsCoordinator(ctx context.Context, projectID int64, userID int64) (bool, error) {
	project := r.projects[projectID]
	return project != nil && project.PCUserID != nil && *project.PCUserID == userID, nil
}

func (r *fakeProjectRepo) Update(ctx context.Context, tx *gorm.DB, id int64, updates map[string]any) error {
	project := r.projects[id]
	if name, ok := updates["name"].(string); ok {
		project.Name = name
	}
	if budget, ok := updates["budget"].(string); ok {
		project.Budget = budget
	}
	if pmUserID, ok := updates["pm_user_id"].(int64); ok {
		project.PMUserID = pmUserID
	}
	if pcUserID, present := updates["pc_user_id"]; present {
		project.PCUserID = nil
		if id, ok := pcUserID.(int64); ok {
			project.PCUserID = &id
		}
	}
	if status, ok := updates["status"].(model.ProjectStatus); ok {
		project.Status = status
	}
	return nil
}

func (r *fakeProjectRepo) HasUnclosedAdvances(context.Context, *gorm.DB, int64) (bool, error) {
	return false, nil
}

func (r *fakeProjectRepo) FindAssignableUsers(ctx context.Context, _ *gorm.DB, userIDs []int64) ([]model.AssignableUser, error) {
	var users []model.AssignableUser
	for _, id := range userIDs {
		if user, ok := r.users[id]; ok {
			users = append(users, user)
		}
	}
	return users, nil
}

func (r *fakeProjectRepo) GetMembersByUserIDs(ctx context.Context, userIDs []int64) ([]model.ProjectUserSummary, error) {
	var members []model.ProjectUserSummary
	for _, id := range userIDs {
		if _, ok := r.users[id]; ok {
			members = append(members, model.ProjectUserSummary{UserID: id})
		}
	}
	return members, nil
}

func int64Ptr(v int64) *int64    { return &v }
func stringPtr(v string) *string { return &v }

func createRequest(pmUserID, pcUserID *int64) *dto.CreateProjectRequest {
	return &dto.CreateProjectRequest{
		Name:     "ERBM Pilot",
		Budget:   "1000.00",
		PMUserID: pmUserID,
		PCUserID: pcUserID,
	}
}

// seedProject creates project 1 with PM 2 and PC 3.
func seedProject(t *testing.T, repo *fakeProjectRepo) *ProjectService {
	t.Helper()
	svc := NewProjectService(repo)
	if _, err := svc.Create(context.Background(), 2, "PM", createRequest(nil, int64Ptr(3))); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return svc
}

func TestProjectCreateByPCAddsCreatorAsSolePC(t *testing.T) {
	repo := newFakeProjectRepo()
	svc := NewProjectService(repo)

	detail, err := svc.Create(context.Background(), 3, "PC", createRequest(int64Ptr(2), nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Project.PMUserID != 2 {
		t.Errorf("pm = %d, want 2", detail.Project.PMUserID)
	}
	if detail.PC == nil || detail.PC.UserID != 3 {
		t.Errorf("pc = %#v, want creator 3", detail.PC)
	}
}

func TestProjectCreateByPMWithoutPC(t *testing.T) {
	repo := newFakeProjectRepo()
	svc := NewProjectService(repo)

	detail, err := svc.Create(context.Background(), 2, "PM", createRequest(nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Project.PMUserID != 2 {
		t.Errorf("pm = %d, want creator 2", detail.Project.PMUserID)
	}
	if detail.PC != nil {
		t.Errorf("pc = %#v, want nil", detail.PC)
	}
}

func TestProjectCreateByPCRequiresPM(t *testing.T) {
	svc := NewProjectService(newFakeProjectRepo())

	_, err := svc.Create(context.Background(), 3, "PC", createRequest(nil, nil))
	if !errors.Is(err, dto.ErrBadRequest) {
		t.Fatalf("err = %v, want %v", err, dto.ErrBadRequest)
	}
}

func TestProjectCreateDuplicateCode(t *testing.T) {
	repo := newFakeProjectRepo()
	repo.createErr = dto.ErrProjectCodeExists
	svc := NewProjectService(repo)

	_, err := svc.Create(context.Background(), 2, "PM", createRequest(nil, nil))
	if !errors.Is(err, dto.ErrProjectCodeExists) {
		t.Fatalf("err = %v, want %v", err, dto.ErrProjectCodeExists)
	}
}

func TestProjectCreateInvalidMembers(t *testing.T) {
	tests := []struct {
		name string
		req  *dto.CreateProjectRequest
		want error
	}{
		{"pm has PC role", createRequest(int64Ptr(3), nil), dto.ErrInvalidProjectManager},
		{"pm does not exist", createRequest(int64Ptr(99), nil), dto.ErrInvalidProjectManager},
		{"pc has PM role", createRequest(int64Ptr(2), int64Ptr(7)), dto.ErrInvalidProjectCoordinator},
		{"pc is not active", createRequest(int64Ptr(2), int64Ptr(6)), dto.ErrInvalidProjectCoordinator},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewProjectService(newFakeProjectRepo())
			_, err := svc.Create(context.Background(), 1, "ADMIN", tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestProjectCreateInvalidBudget(t *testing.T) {
	for _, budget := range []string{"0", "-5", "abc", "10.555"} {
		svc := NewProjectService(newFakeProjectRepo())
		req := createRequest(nil, nil)
		req.Budget = budget

		_, err := svc.Create(context.Background(), 2, "PM", req)
		if !errors.Is(err, dto.ErrBadRequest) {
			t.Errorf("budget %q: err = %v, want %v", budget, err, dto.ErrBadRequest)
		}
	}
}

func TestProjectCreateByAccountingForbidden(t *testing.T) {
	svc := NewProjectService(newFakeProjectRepo())

	_, err := svc.Create(context.Background(), 5, "ACCOUNTING", createRequest(int64Ptr(2), nil))
	if !errors.Is(err, dto.ErrForbidden) {
		t.Fatalf("err = %v, want %v", err, dto.ErrForbidden)
	}
}

func TestProjectGetByIDAccess(t *testing.T) {
	repo := newFakeProjectRepo()
	svc := seedProject(t, repo)

	tests := []struct {
		name   string
		userID int64
		role   string
		want   error
	}{
		{"admin", 1, "ADMIN", nil},
		{"accounting", 5, "ACCOUNTING", nil},
		{"project pm", 2, "PM", nil},
		{"project pc", 3, "PC", nil},
		{"other pc", 4, "PC", dto.ErrForbidden},
		{"other pm", 7, "PM", dto.ErrForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GetByID(context.Background(), tt.userID, tt.role, 1)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestProjectListOnlyShowsOwnProjects(t *testing.T) {
	repo := newFakeProjectRepo()
	svc := seedProject(t, repo)

	projects, _, err := svc.List(context.Background(), 4, "PC", model.ProjectFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("other pc sees %d projects, want 0", len(projects))
	}

	projects, _, err = svc.List(context.Background(), 5, "ACCOUNTING", model.ProjectFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("accounting sees %d projects, want 1", len(projects))
	}
}

func TestProjectUpdatePermissions(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
		role   string
		req    *dto.UpdateProjectRequest
		want   error
	}{
		{"pc can rename", 3, "PC", &dto.UpdateProjectRequest{Name: stringPtr("New name")}, nil},
		{"pc cannot change budget", 3, "PC", &dto.UpdateProjectRequest{Budget: stringPtr("2000")}, dto.ErrForbidden},
		{"pc cannot close", 3, "PC", &dto.UpdateProjectRequest{Status: stringPtr("CLOSED")}, dto.ErrForbidden},
		{"other pc cannot rename", 4, "PC", &dto.UpdateProjectRequest{Name: stringPtr("New name")}, dto.ErrForbidden},
		{"pm can change budget", 2, "PM", &dto.UpdateProjectRequest{Budget: stringPtr("2000")}, nil},
		{"admin can change pm", 1, "ADMIN", &dto.UpdateProjectRequest{PMUserID: int64Ptr(7)}, nil},
		{"admin cannot close", 1, "ADMIN", &dto.UpdateProjectRequest{Status: stringPtr("CLOSED")}, dto.ErrForbidden},
		{"accounting cannot update", 5, "ACCOUNTING", &dto.UpdateProjectRequest{Name: stringPtr("x")}, dto.ErrForbidden},
		{"invalid new pm", 2, "PM", &dto.UpdateProjectRequest{PMUserID: int64Ptr(3)}, dto.ErrInvalidProjectManager},
		{"empty body", 2, "PM", &dto.UpdateProjectRequest{}, dto.ErrBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeProjectRepo()
			svc := seedProject(t, repo)

			_, err := svc.Update(context.Background(), tt.userID, tt.role, 1, tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestProjectCloseIsPermanent(t *testing.T) {
	repo := newFakeProjectRepo()
	svc := seedProject(t, repo)

	detail, err := svc.Update(context.Background(), 2, "PM", 1, &dto.UpdateProjectRequest{Status: stringPtr("CLOSED")})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if detail.Project.Status != model.ProjectStatusClosed {
		t.Fatalf("status = %s, want CLOSED", detail.Project.Status)
	}

	_, err = svc.Update(context.Background(), 2, "PM", 1, &dto.UpdateProjectRequest{Name: stringPtr("again")})
	if !errors.Is(err, dto.ErrProjectClosed) {
		t.Fatalf("err = %v, want %v", err, dto.ErrProjectClosed)
	}
}
