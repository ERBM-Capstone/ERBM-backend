package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type projectContractRepo struct {
	*repository.ProjectRepository
	project model.Project
}

func (r *projectContractRepo) WithTx(_ context.Context, fn func(*gorm.DB) error) error {
	return fn(nil)
}

func (r *projectContractRepo) NextProjectNumber(context.Context, *gorm.DB, int) (int64, error) {
	return 1, nil
}

func (r *projectContractRepo) Create(_ context.Context, _ *gorm.DB, project *model.Project) error {
	project.ID = 1
	r.project = *project
	return nil
}

func (r *projectContractRepo) GetByID(context.Context, int64) (*model.Project, error) {
	return &r.project, nil
}

func (r *projectContractRepo) GetByIDForUpdate(context.Context, *gorm.DB, int64) (*model.Project, error) {
	return &r.project, nil
}

func (r *projectContractRepo) List(context.Context, model.ProjectFilter) ([]*model.Project, int64, error) {
	return []*model.Project{&r.project}, 1, nil
}

func (r *projectContractRepo) Update(_ context.Context, _ *gorm.DB, _ int64, updates map[string]any) error {
	if name, ok := updates["name"].(string); ok {
		r.project.Name = name
	}
	return nil
}

func (r *projectContractRepo) FindAssignableUsers(context.Context, *gorm.DB, []int64) ([]model.AssignableUser, error) {
	return []model.AssignableUser{{UserID: 100, RoleCode: "PM", Status: model.UserStatusActive}}, nil
}

func (r *projectContractRepo) GetMembersByUserIDs(context.Context, []int64) ([]model.ProjectUserSummary, error) {
	return []model.ProjectUserSummary{{UserID: 100, FirstName: "Project", LastName: "Manager"}}, nil
}

func TestProjectAPIWithoutBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &projectContractRepo{}
	h := handler.NewProjectHandler(service.NewProjectService(repo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, int64(100))
		c.Set(middleware.ContextKeyRole, "PM")
	})
	router.POST("/api/v1/projects", h.Create)
	router.PATCH("/api/v1/projects/:id", h.Update)
	router.GET("/api/v1/projects/:id", h.GetByID)
	router.GET("/api/v1/projects", h.List)

	for _, tt := range []struct {
		name, method, path, body, wantName string
		status                             int
		list                               bool
	}{
		{"create", http.MethodPost, "/api/v1/projects", `{"name":"Project without budget"}`, "Project without budget", http.StatusCreated, false},
		{"update", http.MethodPatch, "/api/v1/projects/1", `{"name":"Renamed project"}`, "Renamed project", http.StatusOK, false},
		{"detail", http.MethodGet, "/api/v1/projects/1", "", "Renamed project", http.StatusOK, false},
		{"list", http.MethodGet, "/api/v1/projects", "", "Renamed project", http.StatusOK, true},
	} {
		if !t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tt.status, response.Body.String())
			}
			var envelope struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			payload := envelope.Data
			if tt.list {
				var items []json.RawMessage
				if err := json.Unmarshal(payload, &items); err != nil {
					t.Fatal(err)
				}
				if len(items) != 1 {
					t.Fatalf("projects=%d want=1", len(items))
				}
				payload = items[0]
			}
			var project map[string]json.RawMessage
			if err := json.Unmarshal(payload, &project); err != nil {
				t.Fatal(err)
			}
			if _, exists := project["budget"]; exists {
				t.Fatalf("removed budget field is still returned: %s", payload)
			}
			var name, code string
			if err := json.Unmarshal(project["name"], &name); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(project["project_code"], &code); err != nil {
				t.Fatal(err)
			}
			if name != tt.wantName || !strings.HasPrefix(code, "PRJ-") || repo.project.Name != tt.wantName {
				t.Fatalf("name=%q code=%q saved name=%q", name, code, repo.project.Name)
			}
		}) {
			t.FailNow()
		}
	}
}
