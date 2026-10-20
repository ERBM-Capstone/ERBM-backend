package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type projectCodeRepo struct {
	*singleCoordinatorRepo
	tx            *gorm.DB
	number        int64
	year          int
	allocationErr error
	allocations   int
}

func (r *projectCodeRepo) WithTx(_ context.Context, fn func(*gorm.DB) error) error {
	r.tx = &gorm.DB{}
	return fn(r.tx)
}

func (r *projectCodeRepo) NextProjectNumber(_ context.Context, tx *gorm.DB, year int) (int64, error) {
	r.allocations++
	if tx == nil || tx != r.tx {
		return 0, errors.New("allocation must use the creation transaction")
	}
	if r.allocationErr != nil {
		return 0, r.allocationErr
	}
	r.year = year
	r.number++
	return r.number, nil
}

func TestProjectHTTPGeneratesCodeWithoutClientInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, clientCode := range []string{"", `,"project_code":"CLIENT-CODE"`} {
		t.Run(clientCode, func(t *testing.T) {
			repo := &projectCodeRepo{singleCoordinatorRepo: newSingleCoordinatorRepo(), number: 40}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextKeyUserID, int64(100))
				c.Set(middleware.ContextKeyRole, "PM")
			})
			router.POST("/projects", handler.NewProjectHandler(service.NewProjectService(repo)).Create)
			for _, suffix := range []string{"000041", "000042"} {
				request := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{"name":"  Automatic code  ","budget":"1000","pc_user_id":25`+clientCode+`}`))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusCreated {
					t.Fatalf("create without a client-controlled code: status=%d body=%s", response.Code, response.Body.String())
				}
				var body struct {
					Data dto.ProjectResponse `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Data.ProjectCode == "CLIENT-CODE" {
					t.Fatal("the generated code must ignore client-supplied project_code")
				}
				createdAt, err := time.Parse(time.RFC3339, body.Data.CreatedAt)
				if err != nil || createdAt.IsZero() {
					t.Fatalf("invalid created_at=%q error=%v", body.Data.CreatedAt, err)
				}
				year := createdAt.In(time.FixedZone("UTC+7", 7*60*60)).Year()
				want := fmt.Sprintf("PRJ-%d-%s", year, suffix)
				if body.Data.ProjectCode != want || repo.project.ProjectCode != want || repo.year != year {
					t.Fatalf("response code=%q stored=%q counter year=%d want=%q", body.Data.ProjectCode, repo.project.ProjectCode, repo.year, want)
				}
				if body.Data.Name != "Automatic code" || body.Data.PM == nil || body.Data.PM.UserID != 100 || body.Data.PC == nil || body.Data.PC.UserID != 25 {
					t.Fatalf("unexpected Project response: %#v", body.Data)
				}
			}
		})
	}
}

func TestProjectCodeAllocationFailureDoesNotCreateProject(t *testing.T) {
	repo := &projectCodeRepo{singleCoordinatorRepo: newSingleCoordinatorRepo(), allocationErr: errors.New("counter unavailable")}
	detail, err := service.NewProjectService(repo).Create(context.Background(), 100, "PM", &dto.CreateProjectRequest{Name: "Project", Budget: "1000"})
	if !errors.Is(err, dto.ErrInternalServer) || detail != nil || repo.created != 0 || repo.allocations != 1 {
		t.Fatalf("detail=%v error=%v writes=%d allocations=%d", detail, err, repo.created, repo.allocations)
	}
}

func TestProjectInvalidRequestsDoNotAllocateCodes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		actor int64
		role  string
		req   *dto.CreateProjectRequest
		want  error
	}{
		{"unauthenticated", 0, "PM", &dto.CreateProjectRequest{Name: "Project", Budget: "1000"}, dto.ErrUnauthorized},
		{"forbidden", 100, "ACCOUNTING", &dto.CreateProjectRequest{Name: "Project", Budget: "1000"}, dto.ErrForbidden},
		{"nil request", 100, "PM", nil, dto.ErrBadRequest},
		{"blank name", 100, "PM", &dto.CreateProjectRequest{Name: "  ", Budget: "1000"}, dto.ErrBadRequest},
		{"invalid budget", 100, "PM", &dto.CreateProjectRequest{Name: "Project", Budget: "0"}, dto.ErrBadRequest},
		{"invalid manager", 999, "PM", &dto.CreateProjectRequest{Name: "Project", Budget: "1000"}, dto.ErrInvalidProjectManager},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &projectCodeRepo{singleCoordinatorRepo: newSingleCoordinatorRepo()}
			detail, err := service.NewProjectService(repo).Create(context.Background(), tt.actor, tt.role, tt.req)
			if !errors.Is(err, tt.want) || detail != nil || repo.created != 0 || repo.allocations != 0 {
				t.Fatalf("detail=%v error=%v writes=%d allocations=%d", detail, err, repo.created, repo.allocations)
			}
		})
	}
}

type postmanProjectRepo struct{ *projectCodeRepo }

func (r *postmanProjectRepo) FindAssignableUsers(context.Context, *gorm.DB, []int64) ([]model.AssignableUser, error) {
	return []model.AssignableUser{
		{UserID: 40, RoleCode: "PM", Status: model.UserStatusActive},
		{UserID: 48, RoleCode: "PC", Status: model.UserStatusActive},
		{UserID: 25, RoleCode: "PC", Status: model.UserStatusActive},
	}, nil
}

func TestProjectPostmanPayloadUsesAuthenticatedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const payload = `{
		"name": "ERBM Development Project",
		"description": "Development of ERBM System",
		"budget": "1500000.00",
		"start_date": "2026-10-18",
		"end_date": "2027-03-31",
		"pm_user_id": 40`
	for _, tt := range []struct {
		name   string
		actor  int64
		role   string
		status int
		autoPC bool
		wantPC int64
	}{
		{"ADMIN assigns PM and PC", 1, "ADMIN", http.StatusCreated, false, 48},
		{"PM creates own Project", 40, "PM", http.StatusCreated, false, 48},
		{"PC assigns self", 48, "PC", http.StatusCreated, false, 48},
		{"PC cannot assign another PC", 25, "PC", http.StatusBadRequest, false, 0},
		{"PC 25 is assigned automatically", 25, "PC", http.StatusCreated, true, 25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &postmanProjectRepo{&projectCodeRepo{singleCoordinatorRepo: newSingleCoordinatorRepo()}}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextKeyUserID, tt.actor)
				c.Set(middleware.ContextKeyRole, tt.role)
			})
			router.POST("/api/v1/projects", handler.NewProjectHandler(service.NewProjectService(repo)).Create)
			bodyJSON := payload
			if !tt.autoPC {
				bodyJSON += `,"pc_user_id":48`
			}
			bodyJSON += `}`
			request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(bodyJSON))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tt.status, response.Body.String())
			}
			if tt.status == http.StatusCreated {
				var body struct {
					Data dto.ProjectResponse `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Data.PM == nil || body.Data.PM.UserID != 40 || body.Data.PC == nil || body.Data.PC.UserID != tt.wantPC ||
					body.Data.Budget != "1500000.00" || body.Data.StartDate == nil || *body.Data.StartDate != "2026-10-18" ||
					body.Data.EndDate == nil || *body.Data.EndDate != "2027-03-31" {
					t.Fatalf("unexpected response: %#v", body.Data)
				}
			} else if repo.created != 0 || repo.allocations != 0 {
				t.Fatalf("invalid request wrote a Project: writes=%d allocations=%d", repo.created, repo.allocations)
			}
		})
	}
}
