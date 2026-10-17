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
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type singleCoordinatorRepo struct {
	*repository.ProjectRepository // Unexpected persistence calls fail instead of hiding missing behavior.
	project                       model.Project
	created, updated              int
}

func newSingleCoordinatorRepo() *singleCoordinatorRepo {
	return &singleCoordinatorRepo{project: model.Project{ID: 1, ProjectCode: "PRJ001", Name: "Project", Budget: "1000.00", PMUserID: 100, Status: model.ProjectStatusActive}}
}
func (r *singleCoordinatorRepo) WithTx(_ context.Context, fn func(*gorm.DB) error) error {
	return fn(&gorm.DB{})
}
func (r *singleCoordinatorRepo) NextProjectNumber(context.Context, *gorm.DB, int) (int64, error) {
	return int64(r.created + 1), nil
}
func (r *singleCoordinatorRepo) Create(_ context.Context, _ *gorm.DB, p *model.Project) error {
	r.created++
	p.ID = 1
	r.project = *p
	return nil
}
func (r *singleCoordinatorRepo) GetByID(context.Context, int64) (*model.Project, error) {
	return &r.project, nil
}
func (r *singleCoordinatorRepo) GetByIDForUpdate(context.Context, *gorm.DB, int64) (*model.Project, error) {
	return &r.project, nil
}
func (r *singleCoordinatorRepo) IsCoordinator(_ context.Context, _ int64, userID int64) (bool, error) {
	return r.project.PCUserID != nil && *r.project.PCUserID == userID, nil
}
func (r *singleCoordinatorRepo) Update(_ context.Context, _ *gorm.DB, _ int64, updates map[string]any) error {
	r.updated++
	if pc, present := updates["pc_user_id"]; present {
		r.project.PCUserID = nil
		if id, ok := pc.(int64); ok {
			r.project.PCUserID = &id
		}
	}
	if name, ok := updates["name"].(string); ok {
		r.project.Name = name
	}
	return nil
}
func (r *singleCoordinatorRepo) HasUnclosedAdvances(context.Context, *gorm.DB, int64) (bool, error) {
	return false, nil
}
func (r *singleCoordinatorRepo) FindAssignableUsers(context.Context, *gorm.DB, []int64) ([]model.AssignableUser, error) {
	return []model.AssignableUser{
		{UserID: 100, RoleCode: "PM", Status: model.UserStatusActive}, {UserID: 25, RoleCode: "PC", Status: model.UserStatusActive}, {UserID: 26, RoleCode: "PC", Status: model.UserStatusActive},
		{UserID: 27, RoleCode: "PC", Status: model.UserStatusInvited},
	}, nil
}
func (r *singleCoordinatorRepo) GetMembersByUserIDs(_ context.Context, ids []int64) ([]model.ProjectUserSummary, error) {
	var members []model.ProjectUserSummary
	for _, id := range ids {
		members = append(members, model.ProjectUserSummary{UserID: id})
	}
	return members, nil
}

func TestProjectCreatePCCannotAssignAnotherCoordinator(t *testing.T) {
	pm, otherPC := int64(100), int64(26)
	repo := newSingleCoordinatorRepo()
	_, err := service.NewProjectService(repo).Create(context.Background(), 25, "PC", &dto.CreateProjectRequest{Name: "Project", Budget: "1000", PMUserID: &pm, PCUserID: &otherPC})
	if !errors.Is(err, dto.ErrBadRequest) || repo.created != 0 {
		t.Fatalf("error=%v, writes=%d", err, repo.created)
	}
}

func TestProjectCreateKeepsZeroOrOnePCAndAutoAssignsPCCreator(t *testing.T) {
	pm, pc25, pc26 := int64(100), int64(25), int64(26)
	for _, tt := range []struct {
		role  string
		actor int64
		pc    *int64
		want  int64
	}{
		{"ADMIN", 1, nil, 0}, {"ADMIN", 1, &pc26, 26}, {"PM", 100, &pc25, 25}, {"PC", 25, nil, 25}, {"PC", 25, &pc25, 25},
	} {
		repo := newSingleCoordinatorRepo()
		result, err := service.NewProjectService(repo).Create(context.Background(), tt.actor, tt.role, &dto.CreateProjectRequest{Name: "Project", Budget: "1000", PMUserID: &pm, PCUserID: tt.pc})
		if err != nil {
			t.Fatalf("role=%s: %v", tt.role, err)
		}
		if tt.want == 0 {
			if result.PC != nil {
				t.Fatalf("unassigned project=%#v", result.PC)
			}
		} else if result.PC == nil || result.PC.UserID != tt.want {
			t.Fatalf("role=%s: pc=%#v", tt.role, result.PC)
		}
	}
}

func TestProjectHTTPRejectsMoreThanOnePC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newSingleCoordinatorRepo()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, int64(100))
		c.Set(middleware.ContextKeyRole, "PM")
		c.Next()
	})
	h := handler.NewProjectHandler(service.NewProjectService(repo))
	r.POST("/projects", h.Create)
	r.PATCH("/projects/:id", h.Update)
	for _, tt := range []struct{ method, path, body string }{
		{http.MethodPost, "/projects", `{"name":"Project","budget":"1000","pc_user_id":[25,26]}`},
		{http.MethodPatch, "/projects/1", `{"pc_user_id":[25,26]}`},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest || repo.created != 0 || repo.updated != 0 {
			t.Fatalf("%s: status=%d, body=%s", tt.method, w.Code, w.Body.String())
		}
	}
}

func TestProjectCreateHTTPUsesSinglePCUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name   string
		role   string
		actor  int64
		pcJSON string
		wantPC int64
		status int
	}{
		{"admin assigns PC", "ADMIN", 1, `25`, 25, http.StatusCreated},
		{"PM assigns PC", "PM", 100, `26`, 26, http.StatusCreated},
		{"admin omits PC", "ADMIN", 1, ``, 0, http.StatusCreated},
		{"admin sends null", "ADMIN", 1, `null`, 0, http.StatusCreated},
		{"PC is auto assigned", "PC", 25, ``, 25, http.StatusCreated},
		{"PC supplies own ID", "PC", 25, `25`, 25, http.StatusCreated},
		{"PC cannot assign another PC", "PC", 25, `26`, 0, http.StatusBadRequest},
		{"zero ID", "ADMIN", 1, `0`, 0, http.StatusBadRequest},
		{"negative ID", "ADMIN", 1, `-1`, 0, http.StatusBadRequest},
		{"array of one", "ADMIN", 1, `[25]`, 0, http.StatusBadRequest},
		{"array of two", "ADMIN", 1, `[25,26]`, 0, http.StatusBadRequest},
		{"string ID", "ADMIN", 1, `"25"`, 0, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newSingleCoordinatorRepo()
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(middleware.ContextKeyUserID, tt.actor)
				c.Set(middleware.ContextKeyRole, tt.role)
				c.Next()
			})
			r.POST("/projects", handler.NewProjectHandler(service.NewProjectService(repo)).Create)
			w := httptest.NewRecorder()
			body := `{"name":"Project","budget":"1000","pm_user_id":100`
			if tt.pcJSON != "" {
				body += `,"pc_user_id":` + tt.pcJSON
			}
			body += `}`
			req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Fatalf("status=%d, want=%d: %s", w.Code, tt.status, w.Body.String())
			}
			if tt.status != http.StatusCreated {
				if repo.created != 0 {
					t.Fatalf("invalid request wrote a Project: %d", repo.created)
				}
				return
			}
			var response struct {
				Data dto.ProjectResponse `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if tt.wantPC == 0 {
				if response.Data.PC != nil {
					t.Fatalf("unassigned Project has PC: %#v", response.Data.PC)
				}
			} else if response.Data.PC == nil || response.Data.PC.UserID != tt.wantPC {
				t.Fatalf("PC=%#v, want user %d", response.Data.PC, tt.wantPC)
			}
		})
	}
}

func TestProjectPatchPCDistinguishesOmissionNullAndID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name, role, body string
		actor            int64
		status           int
		wantPC           int64
	}{
		{"omitted preserves PC", "PM", `{"name":"Renamed"}`, 100, http.StatusOK, 25},
		{"null removes PC", "PM", `{"pc_user_id":null}`, 100, http.StatusOK, 0},
		{"ID replaces PC", "PM", `{"pc_user_id":26}`, 100, http.StatusOK, 26},
		{"same PC accepted", "PM", `{"pc_user_id":25}`, 100, http.StatusOK, 25},
		{"admin removes PC", "ADMIN", `{"pc_user_id":null}`, 1, http.StatusOK, 0},
		{"PC cannot remove itself", "PC", `{"pc_user_id":null}`, 25, http.StatusForbidden, 25},
		{"PC cannot replace itself", "PC", `{"pc_user_id":26}`, 25, http.StatusForbidden, 25},
		{"PC edits name", "PC", `{"name":"Renamed"}`, 25, http.StatusOK, 25},
		{"foreign PM cannot assign", "PM", `{"pc_user_id":26}`, 999, http.StatusForbidden, 25},
		{"zero", "PM", `{"pc_user_id":0}`, 100, http.StatusBadRequest, 25},
		{"negative", "PM", `{"pc_user_id":-1}`, 100, http.StatusBadRequest, 25},
		{"array", "PM", `{"pc_user_id":[25]}`, 100, http.StatusBadRequest, 25},
		{"string", "PM", `{"pc_user_id":"25"}`, 100, http.StatusBadRequest, 25},
		{"wrong role", "PM", `{"pc_user_id":100}`, 100, http.StatusBadRequest, 25},
		{"inactive PC", "PM", `{"pc_user_id":27}`, 100, http.StatusBadRequest, 25},
		{"missing PC", "PM", `{"pc_user_id":99}`, 100, http.StatusBadRequest, 25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newSingleCoordinatorRepo()
			pcID := int64(25)
			repo.project.PCUserID = &pcID
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(middleware.ContextKeyUserID, tt.actor)
				c.Set(middleware.ContextKeyRole, tt.role)
				c.Next()
			})
			r.PATCH("/projects/:id", handler.NewProjectHandler(service.NewProjectService(repo)).Update)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Fatalf("status=%d, want=%d: %s", w.Code, tt.status, w.Body.String())
			}
			if tt.status != http.StatusOK {
				if repo.updated != 0 {
					t.Fatal("denied PATCH changed the Project")
				}
				return
			}
			var response struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			pc, ok := response.Data["pc"]
			if !ok || response.Data["pcs"] != nil {
				t.Fatalf("response must contain a single pc: %s", w.Body.String())
			}
			if tt.wantPC == 0 {
				if string(pc) != "null" {
					t.Fatalf("removed PC=%s", pc)
				}
			} else {
				var member struct {
					UserID int64 `json:"user_id"`
				}
				if err := json.Unmarshal(pc, &member); err != nil || member.UserID != tt.wantPC {
					t.Fatalf("PC=%s, want %d: %v", pc, tt.wantPC, err)
				}
			}
		})
	}
}
