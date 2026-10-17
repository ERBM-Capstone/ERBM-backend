package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type reassignedProjectRepo struct {
	*singleCoordinatorRepo
	memberError error
}

func (r *reassignedProjectRepo) readThenReassign() *model.Project {
	loaded := r.project
	newPC := int64(26)
	r.project.PMUserID, r.project.PCUserID = 200, &newPC
	return &loaded
}

func (r *reassignedProjectRepo) GetByID(context.Context, int64) (*model.Project, error) {
	return r.readThenReassign(), nil
}

func (r *reassignedProjectRepo) List(context.Context, model.ProjectFilter) ([]*model.Project, int64, error) {
	return []*model.Project{r.readThenReassign()}, 1, nil
}

func (r *reassignedProjectRepo) GetMembersByUserIDs(_ context.Context, ids []int64) ([]model.ProjectUserSummary, error) {
	if r.memberError != nil {
		return nil, r.memberError
	}
	profiles := map[int64]model.ProjectUserSummary{
		100: {UserID: 100, FirstName: "Original", LastName: "Manager", Email: "original-pm@example.test"},
		200: {UserID: 200, FirstName: "Replacement", LastName: "Manager", Email: "replacement-pm@example.test"},
		25:  {UserID: 25, FirstName: "Original", LastName: "Coordinator", Email: "original-pc@example.test"},
		26:  {UserID: 26, FirstName: "Replacement", LastName: "Coordinator", Email: "replacement-pc@example.test"},
	}
	var members []model.ProjectUserSummary
	for _, id := range ids {
		if member, ok := profiles[id]; ok {
			members = append(members, member)
		}
	}
	return members, nil
}

func TestProjectDetailsKeepLoadedAssignmentsDuringReassignment(t *testing.T) {
	for _, operation := range []string{"list", "detail"} {
		for _, assignedPC := range []bool{false, true} {
			name := operation + "/without PC"
			if assignedPC {
				name = operation + "/with PC"
			}
			t.Run(name, func(t *testing.T) {
				repo := &reassignedProjectRepo{singleCoordinatorRepo: newSingleCoordinatorRepo()}
				if assignedPC {
					pc := int64(25)
					repo.project.PCUserID = &pc
				}
				svc := service.NewProjectService(repo)
				var detail *model.ProjectDetail
				var err error
				if operation == "list" {
					items, total, listErr := svc.List(context.Background(), 1, "ADMIN", model.ProjectFilter{Page: 1, PageSize: 20})
					if listErr != nil || total != 1 || len(items) != 1 {
						t.Fatalf("items=%v total=%d error=%v", items, total, listErr)
					}
					detail = items[0]
				} else {
					detail, err = svc.GetByID(context.Background(), 100, "PM", 1)
				}
				if err != nil || detail == nil || detail.PM == nil || detail.Project.PMUserID != 100 || detail.PM.UserID != 100 {
					t.Fatalf("loaded PM=100 must match returned PM: detail=%#v error=%v", detail, err)
				}
				response := dto.ToProjectResponse(detail)
				if response.PM.FirstName != "Original" || response.PM.LastName != "Manager" || response.PM.Email != "original-pm@example.test" {
					t.Fatalf("wrong PM profile: %#v", response.PM)
				}
				if assignedPC {
					if response.PC == nil || response.PC.UserID != 25 || response.PC.FirstName != "Original" || response.PC.LastName != "Coordinator" || response.PC.Email != "original-pc@example.test" {
						t.Fatalf("loaded PC=25 must match returned profile: %#v", response.PC)
					}
				} else if response.PC != nil {
					t.Fatalf("unassigned Project received a newly assigned PC: %#v", response.PC)
				}
			})
		}
	}
}

func TestProjectMemberLookupFailureReturnsInternalError(t *testing.T) {
	repo := &reassignedProjectRepo{singleCoordinatorRepo: newSingleCoordinatorRepo(), memberError: errors.New("lookup unavailable")}
	detail, err := service.NewProjectService(repo).GetByID(context.Background(), 100, "PM", 1)
	if !errors.Is(err, dto.ErrInternalServer) || detail != nil {
		t.Fatalf("detail=%v error=%v", detail, err)
	}
}

type projectDetailReadRepo struct {
	*singleCoordinatorRepo
	projectError, memberError error
	projectReads, memberReads int
}

func (r *projectDetailReadRepo) GetByID(ctx context.Context, id int64) (*model.Project, error) {
	r.projectReads++
	if r.projectError != nil {
		return nil, r.projectError
	}
	return r.singleCoordinatorRepo.GetByID(ctx, id)
}

func (r *projectDetailReadRepo) GetMembersByUserIDs(ctx context.Context, ids []int64) ([]model.ProjectUserSummary, error) {
	r.memberReads++
	if r.memberError != nil {
		return nil, r.memberError
	}
	return r.singleCoordinatorRepo.GetMembersByUserIDs(ctx, ids)
}

func TestProjectSingleDetailReadBehavior(t *testing.T) {
	for _, operation := range []string{"get", "create", "patch"} {
		for _, tt := range []struct {
			name                      string
			projectError, memberError error
		}{
			{"success", nil, nil},
			{"project not found", dto.ErrProjectNotFound, nil},
			{"project lookup fails", errors.New("project lookup unavailable"), nil},
			{"member lookup fails", nil, errors.New("member lookup unavailable")},
		} {
			t.Run(operation+"/"+tt.name, func(t *testing.T) {
				repo := &projectDetailReadRepo{singleCoordinatorRepo: newSingleCoordinatorRepo(), projectError: tt.projectError, memberError: tt.memberError}
				pc := int64(25)
				repo.project.PCUserID = &pc
				svc := service.NewProjectService(repo)
				var detail *model.ProjectDetail
				var err error
				switch operation {
				case "get":
					detail, err = svc.GetByID(context.Background(), 100, "PM", 1)
				case "create":
					detail, err = svc.Create(context.Background(), 100, "PM", &dto.CreateProjectRequest{Name: "Project", Budget: "1000", PCUserID: &pc})
				case "patch":
					name := "Renamed"
					detail, err = svc.Update(context.Background(), 100, "PM", 1, &dto.UpdateProjectRequest{Name: &name})
				}
				var want error
				if tt.projectError != nil || tt.memberError != nil {
					want = dto.ErrInternalServer
					if operation == "get" && tt.projectError == dto.ErrProjectNotFound {
						want = dto.ErrProjectNotFound
					}
				}
				if !errors.Is(err, want) || (want != nil && detail != nil) {
					t.Fatalf("detail=%v error=%v want=%v", detail, err, want)
				}
				if want == nil && (detail == nil || detail.Project.ID != 1 || detail.PM == nil || detail.PM.UserID != 100 || detail.PC == nil || detail.PC.UserID != 25) {
					t.Fatalf("incomplete detail: %#v", detail)
				}
				wantMemberReads := 1
				if tt.projectError != nil {
					wantMemberReads = 0
				}
				if repo.projectReads != 1 || repo.memberReads != wantMemberReads {
					t.Fatalf("project reads=%d member reads=%d want=1/%d", repo.projectReads, repo.memberReads, wantMemberReads)
				}
			})
		}
	}
	t.Run("unauthorized member lookup is skipped", func(t *testing.T) {
		repo := &projectDetailReadRepo{singleCoordinatorRepo: newSingleCoordinatorRepo(), memberError: errors.New("profile data must not be read")}
		detail, err := service.NewProjectService(repo).GetByID(context.Background(), 999, "PM", 1)
		if !errors.Is(err, dto.ErrForbidden) || detail != nil || repo.memberReads != 0 {
			t.Fatalf("detail=%v error=%v member reads=%d", detail, err, repo.memberReads)
		}
	})
}

func TestProjectPatchChecksPermissionsBeforeClosedStatus(t *testing.T) {
	name, budget, closed := "Updated", "2000", "CLOSED"
	for _, tt := range []struct {
		name  string
		actor int64
		role  string
		req   dto.UpdateProjectRequest
	}{
		{"foreign PC", 999, "PC", dto.UpdateProjectRequest{Name: &name}},
		{"foreign PM", 999, "PM", dto.UpdateProjectRequest{Name: &name}},
		{"PC edits budget", 25, "PC", dto.UpdateProjectRequest{Budget: &budget}},
		{"PC closes", 25, "PC", dto.UpdateProjectRequest{Status: &closed}},
		{"admin closes", 1, "ADMIN", dto.UpdateProjectRequest{Status: &closed}},
		{"accounting edits", 2, "ACCOUNTING", dto.UpdateProjectRequest{Name: &name}},
	} {
		for _, status := range []model.ProjectStatus{model.ProjectStatusActive, model.ProjectStatusClosed} {
			t.Run(tt.name+"/"+string(status), func(t *testing.T) {
				repo := newSingleCoordinatorRepo()
				pc := int64(25)
				repo.project.PCUserID, repo.project.Status = &pc, status
				detail, err := service.NewProjectService(repo).Update(context.Background(), tt.actor, tt.role, 1, &tt.req)
				if !errors.Is(err, dto.ErrForbidden) || detail != nil || repo.updated != 0 {
					t.Fatalf("detail=%v error=%v writes=%d", detail, err, repo.updated)
				}
			})
		}
	}
}

func TestProjectPatchRecognizesPCCasingForNullAndIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name, role, body string
		actor            int64
		status           int
		wantPC           int64
	}{
		{"upper null", "PM", `{"name":"Renamed","PC_USER_ID":null}`, 100, 200, 0},
		{"mixed null", "PM", `{"pC_uSeR_iD":null}`, 100, 200, 0},
		{"upper ID", "PM", `{"PC_USER_ID":26}`, 100, 200, 26},
		{"last null wins", "PM", `{"pc_user_id":26,"PC_USER_ID":null}`, 100, 200, 0},
		{"last ID wins", "PM", `{"PC_USER_ID":null,"pc_user_id":26}`, 100, 200, 26},
		{"PC cannot clear using upper key", "PC", `{"name":"Renamed","PC_USER_ID":null}`, 25, 403, 25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newSingleCoordinatorRepo()
			pc := int64(25)
			repo.project.PCUserID = &pc
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(middleware.ContextKeyUserID, tt.actor)
				c.Set(middleware.ContextKeyRole, tt.role)
			})
			r.PATCH("/projects/:id", handler.NewProjectHandler(service.NewProjectService(repo)).Update)
			request := httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tt.status, response.Body.String())
			}
			actualPC := int64(0)
			if repo.project.PCUserID != nil {
				actualPC = *repo.project.PCUserID
			}
			if actualPC != tt.wantPC {
				t.Fatalf("persisted PC=%d want=%d", actualPC, tt.wantPC)
			}
			if tt.status == http.StatusOK {
				var body struct {
					Data dto.ProjectResponse `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if tt.wantPC == 0 {
					if body.Data.PC != nil {
						t.Fatalf("response retained a removed PC: %#v", body.Data.PC)
					}
				} else if body.Data.PC == nil || body.Data.PC.UserID != tt.wantPC {
					t.Fatalf("response PC=%#v want=%d", body.Data.PC, tt.wantPC)
				}
			} else if repo.updated != 0 {
				t.Fatal("forbidden patch wrote fields")
			}
		})
	}
}

func TestProjectServiceRejectsNilRequests(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("nil request must return BadRequest instead of panicking: %v", recovered)
				}
			}()
			repo := newSingleCoordinatorRepo()
			svc := service.NewProjectService(repo)
			var detail *model.ProjectDetail
			var err error
			if operation == "create" {
				detail, err = svc.Create(context.Background(), 100, "PM", nil)
			} else {
				detail, err = svc.Update(context.Background(), 100, "PM", 1, nil)
			}
			if !errors.Is(err, dto.ErrBadRequest) || detail != nil || repo.created != 0 || repo.updated != 0 {
				t.Fatalf("detail=%v error=%v writes=%d/%d", detail, err, repo.created, repo.updated)
			}
		})
	}
}
