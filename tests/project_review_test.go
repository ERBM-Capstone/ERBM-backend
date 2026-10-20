package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type projectReviewRepo struct {
	*singleCoordinatorRepo
	tx           *gorm.DB
	filter       model.ProjectFilter
	advances     map[int64][]model.AdvanceStatus
	advanceError error
	lastUpdates  map[string]any
}

func newProjectReviewRepo() *projectReviewRepo {
	return &projectReviewRepo{singleCoordinatorRepo: newSingleCoordinatorRepo(), advances: map[int64][]model.AdvanceStatus{}}
}

func (r *projectReviewRepo) WithTx(_ context.Context, fn func(*gorm.DB) error) error {
	r.tx = &gorm.DB{}
	return fn(r.tx)
}

func (r *projectReviewRepo) List(_ context.Context, filter model.ProjectFilter) ([]*model.Project, int64, error) {
	r.filter = filter
	if filter.MemberUserID != nil && *filter.MemberUserID != r.project.PMUserID &&
		(r.project.PCUserID == nil || *filter.MemberUserID != *r.project.PCUserID) {
		return []*model.Project{}, 0, nil
	}
	return []*model.Project{&r.project}, 1, nil
}

func (r *projectReviewRepo) IsCoordinator(context.Context, int64, int64) (bool, error) {
	return false, errors.New("membership must use the already loaded Project")
}

func (r *projectReviewRepo) FindAssignableUsers(ctx context.Context, tx *gorm.DB, ids []int64) ([]model.AssignableUser, error) {
	if tx != r.tx {
		return nil, errors.New("assignment lookup must share the transaction connection")
	}
	return r.singleCoordinatorRepo.FindAssignableUsers(ctx, tx, ids)
}

func (r *projectReviewRepo) HasUnclosedAdvances(_ context.Context, tx *gorm.DB, projectID int64) (bool, error) {
	if tx != r.tx {
		return false, errors.New("closing check must share the Project transaction")
	}
	if r.advanceError != nil {
		return false, r.advanceError
	}
	for _, status := range r.advances[projectID] {
		if status != model.AdvanceStatusClosed {
			return true, nil
		}
	}
	return false, nil
}

func (r *projectReviewRepo) Update(ctx context.Context, tx *gorm.DB, id int64, updates map[string]any) error {
	r.lastUpdates = updates
	if status, ok := updates["status"].(model.ProjectStatus); ok {
		r.project.Status = status
	}
	if closedAt, ok := updates["closed_at"].(time.Time); ok {
		r.project.ClosedAt = &closedAt
	}
	return r.singleCoordinatorRepo.Update(ctx, tx, id, updates)
}

func TestProjectReadsRequireAuthenticatedActor(t *testing.T) {
	for _, role := range []string{"ADMIN", "ACCOUNTING", "PM", "PC"} {
		for _, actor := range []int64{0, -1} {
			svc := service.NewProjectService(newProjectReviewRepo())
			if result, _, err := svc.List(context.Background(), actor, role, model.ProjectFilter{}); !errors.Is(err, dto.ErrUnauthorized) || result != nil {
				t.Errorf("List actor=%d role=%s: result=%v error=%v", actor, role, result, err)
			}
			if result, err := svc.GetByID(context.Background(), actor, role, 1); !errors.Is(err, dto.ErrUnauthorized) || result != nil {
				t.Errorf("GetByID actor=%d role=%s: result=%v error=%v", actor, role, result, err)
			}
		}
	}
}

func TestProjectReadsUseLoadedMembershipAndTrustedScope(t *testing.T) {
	for _, tt := range []struct {
		role  string
		actor int64
		count int64
	}{
		{"ADMIN", 1, 1}, {"ACCOUNTING", 2, 1}, {"PM", 100, 1}, {"PC", 25, 1}, {"PC", 26, 0},
	} {
		t.Run(tt.role+"/"+strconv.FormatInt(tt.actor, 10), func(t *testing.T) {
			repo := newProjectReviewRepo()
			pc, foreign := int64(25), int64(999)
			repo.project.PCUserID = &pc
			svc := service.NewProjectService(repo)
			items, total, err := svc.List(context.Background(), tt.actor, tt.role, model.ProjectFilter{MemberUserID: &foreign})
			if err != nil || total != tt.count || int64(len(items)) != tt.count {
				t.Fatalf("List=%v total=%d error=%v", items, total, err)
			}
			if tt.role == "ADMIN" || tt.role == "ACCOUNTING" {
				if repo.filter.MemberUserID != nil {
					t.Fatal("privileged listing retained caller-supplied membership")
				}
			} else if repo.filter.MemberUserID == nil || *repo.filter.MemberUserID != tt.actor {
				t.Fatal("membership was not derived from the authenticated actor")
			}
			item, err := svc.GetByID(context.Background(), tt.actor, tt.role, 1)
			if tt.count == 0 {
				if !errors.Is(err, dto.ErrForbidden) || item != nil {
					t.Fatalf("foreign detail=%v error=%v", item, err)
				}
			} else if err != nil || item.PC == nil || item.PC.UserID != pc {
				t.Fatalf("assigned detail=%v error=%v", item, err)
			}
		})
	}
}

func TestProjectPatchUsesLockedMembershipAndAssignmentConnection(t *testing.T) {
	name, pc, pm := "Updated", int64(25), int64(100)
	for _, tt := range []struct {
		name  string
		actor int64
		role  string
		req   dto.UpdateProjectRequest
	}{
		{"PC renames", pc, "PC", dto.UpdateProjectRequest{Name: &name}},
		{"PM reassigns PC", pm, "PM", dto.UpdateProjectRequest{PCUserID: &pc}},
		{"PM reassigns both members", pm, "PM", dto.UpdateProjectRequest{PMUserID: &pm, PCUserID: &pc}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newProjectReviewRepo()
			repo.project.PCUserID = &pc
			result, err := service.NewProjectService(repo).Update(context.Background(), tt.actor, tt.role, 1, &tt.req)
			if err != nil || result == nil || repo.updated != 1 {
				t.Fatalf("result=%v error=%v writes=%d", result, err, repo.updated)
			}
		})
	}
}

func TestProjectPatchRejectsUnsupportedStatusWithoutWritingOtherFields(t *testing.T) {
	for _, status := range []string{"ACTIVE", "UNKNOWN", ""} {
		repo := newProjectReviewRepo()
		name := "Must not be saved"
		result, err := service.NewProjectService(repo).Update(context.Background(), 100, "PM", 1, &dto.UpdateProjectRequest{Name: &name, Status: &status})
		if !errors.Is(err, dto.ErrBadRequest) || result != nil || repo.updated != 0 || repo.project.Name != "Project" {
			t.Errorf("status=%q result=%v error=%v writes=%d", status, result, err, repo.updated)
		}
	}
}

func TestProjectCloseRequiresEveryAdvanceToBeClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []model.AdvanceStatus{
		model.AdvanceStatusDraft, model.AdvanceStatusPendingPMApproval, model.AdvanceStatusReturnedForRevision,
		model.AdvanceStatusApproved, model.AdvanceStatusActive, model.AdvanceStatusClosed, "",
	} {
		t.Run(string(status), func(t *testing.T) {
			repo := newProjectReviewRepo()
			if status != "" {
				repo.advances[1] = []model.AdvanceStatus{model.AdvanceStatusClosed, status}
			}
			// A different Project's DRAFT must not block this Project.
			repo.advances[2] = []model.AdvanceStatus{model.AdvanceStatusDraft}
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(middleware.ContextKeyUserID, int64(100))
				c.Set(middleware.ContextKeyRole, "PM")
			})
			r.PATCH("/projects/:id", handler.NewProjectHandler(service.NewProjectService(repo)).Update)
			request := httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(`{"status":"CLOSED","name":"Closed project"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if status != "" && status != model.AdvanceStatusClosed {
				if response.Code != http.StatusConflict || repo.updated != 0 || repo.project.Status != model.ProjectStatusActive || repo.project.Name != "Project" {
					t.Fatalf("unclosed %s: status=%d writes=%d body=%s", status, response.Code, repo.updated, response.Body.String())
				}
			} else if response.Code != http.StatusOK || repo.updated != 1 || repo.project.Status != model.ProjectStatusClosed || repo.project.ClosedAt == nil || repo.lastUpdates["closed_by"] != int64(100) {
				t.Fatalf("closable Project: status=%d writes=%d body=%s", response.Code, repo.updated, response.Body.String())
			}
		})
	}
}

func TestProjectCloseStopsWhenAdvanceLookupFails(t *testing.T) {
	repo := newProjectReviewRepo()
	repo.advanceError = errors.New("database unavailable")
	closed := "CLOSED"
	result, err := service.NewProjectService(repo).Update(context.Background(), 100, "PM", 1, &dto.UpdateProjectRequest{Status: &closed})
	if !errors.Is(err, dto.ErrInternalServer) || result != nil || repo.updated != 0 {
		t.Fatalf("result=%v error=%v writes=%d", result, err, repo.updated)
	}
}

func TestProjectListRejectsInvalidFilters(t *testing.T) {
	for _, filter := range []model.ProjectFilter{
		{Page: int(^uint(0) >> 1), PageSize: 100}, {Page: -1}, {PageSize: -1}, {PageSize: 101},
		{Status: "UNKNOWN"}, {Query: strings.Repeat("ก", 256)},
	} {
		_, _, err := service.NewProjectService(newProjectReviewRepo()).List(context.Background(), 100, "PM", filter)
		if !errors.Is(err, dto.ErrBadRequest) {
			t.Errorf("filter=%#v error=%v", filter, err)
		}
	}
}

func TestProjectPatchValidatesAllFieldsBeforeWriting(t *testing.T) {
	name, blank, invalidBudget := "Must not be saved", "  ", "0"
	start, end := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
	beforeStart, afterEnd, invalidDate := "2026-10-16", "2027-04-01", "2026-02-30"
	invalidPM, inactivePC := int64(25), int64(27)
	for _, tt := range []struct {
		name string
		req  dto.UpdateProjectRequest
		want error
	}{
		{"empty body", dto.UpdateProjectRequest{}, dto.ErrBadRequest},
		{"blank name", dto.UpdateProjectRequest{Name: &blank}, dto.ErrBadRequest},
		{"invalid budget", dto.UpdateProjectRequest{Name: &name, Budget: &invalidBudget}, dto.ErrBadRequest},
		{"invalid date", dto.UpdateProjectRequest{Name: &name, StartDate: &invalidDate}, dto.ErrBadRequest},
		{"end before stored start", dto.UpdateProjectRequest{Name: &name, EndDate: &beforeStart}, dto.ErrBadRequest},
		{"start after stored end", dto.UpdateProjectRequest{Name: &name, StartDate: &afterEnd}, dto.ErrBadRequest},
		{"invalid PM", dto.UpdateProjectRequest{Name: &name, PMUserID: &invalidPM}, dto.ErrInvalidProjectManager},
		{"inactive PC", dto.UpdateProjectRequest{Name: &name, PCUserID: &inactivePC}, dto.ErrInvalidProjectCoordinator},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newProjectReviewRepo()
			repo.project.StartDate, repo.project.EndDate = &start, &end
			result, err := service.NewProjectService(repo).Update(context.Background(), 100, "PM", 1, &tt.req)
			if !errors.Is(err, tt.want) || result != nil || repo.updated != 0 || repo.project.Name != "Project" {
				t.Fatalf("result=%v error=%v writes=%d", result, err, repo.updated)
			}
		})
	}
}

func TestProjectCreateAndPatchNormalizeNames(t *testing.T) {
	for _, operation := range []string{"create", "patch"} {
		for _, tt := range []struct {
			name, input, want string
		}{
			{"trim whitespace", " \tProject A\n", "Project A"},
			{"preserve internal spaces", "  โครงการ  A  ", "โครงการ  A"},
			{"empty", "", ""},
			{"only whitespace", " \t\n\u2003", ""},
		} {
			t.Run(operation+"/"+tt.name, func(t *testing.T) {
				repo := newSingleCoordinatorRepo()
				svc := service.NewProjectService(repo)
				var detail *model.ProjectDetail
				var err error
				if operation == "create" {
					detail, err = svc.Create(context.Background(), 100, "PM", &dto.CreateProjectRequest{
						Name: tt.input, Budget: "1000",
					})
				} else {
					detail, err = svc.Update(context.Background(), 100, "PM", 1, &dto.UpdateProjectRequest{Name: &tt.input})
				}
				if tt.want == "" {
					if !errors.Is(err, dto.ErrBadRequest) || detail != nil || repo.created+repo.updated != 0 {
						t.Fatalf("invalid name: detail=%v error=%v writes=%d", detail, err, repo.created+repo.updated)
					}
				} else if err != nil || detail == nil || detail.Project.Name != tt.want || repo.project.Name != tt.want {
					t.Fatalf("detail=%v stored name=%q error=%v want=%q", detail, repo.project.Name, err, tt.want)
				}
			})
		}
	}
}

func TestProjectCreateDateRanges(t *testing.T) {
	start, same, later, earlier := "2026-10-18", "2026-10-18", "2026-10-19", "2026-10-17"
	for _, tt := range []struct {
		name       string
		start, end *string
		want       error
	}{
		{"no dates", nil, nil, nil},
		{"only start", &start, nil, nil},
		{"only end", nil, &later, nil},
		{"same day", &start, &same, nil},
		{"end after start", &start, &later, nil},
		{"end before start", &start, &earlier, dto.ErrBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newSingleCoordinatorRepo()
			detail, err := service.NewProjectService(repo).Create(context.Background(), 100, "PM", &dto.CreateProjectRequest{
				Name: "Project", Budget: "1000", StartDate: tt.start, EndDate: tt.end,
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error=%v want=%v", err, tt.want)
			}
			if tt.want != nil {
				if detail != nil || repo.created != 0 {
					t.Fatalf("invalid dates were saved: detail=%v writes=%d", detail, repo.created)
				}
				return
			}
			if detail == nil {
				t.Fatal("missing Project detail")
			}
			for field, date := range map[string]struct {
				got  *time.Time
				want *string
			}{"start_date": {detail.Project.StartDate, tt.start}, "end_date": {detail.Project.EndDate, tt.end}} {
				if date.want == nil {
					if date.got != nil {
						t.Errorf("%s=%v want nil", field, date.got)
					}
				} else if date.got == nil || date.got.Format("2006-01-02") != *date.want {
					t.Errorf("%s=%v want=%s", field, date.got, *date.want)
				}
			}
		})
	}
}

func TestProjectRepositoryKeepsCountAndPageQueriesIndependent(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	var queries []advanceCapturedQuery
	if err := db.Callback().Query().After("gorm:query").Register("capture_project_query", func(db *gorm.DB) {
		queries = append(queries, advanceCapturedQuery{db.Statement.SQL.String(), append([]any(nil), db.Statement.Vars...)})
	}); err != nil {
		t.Fatal(err)
	}
	actor := int64(25)
	_, _, err = repository.NewProjectRepository(db).List(context.Background(), model.ProjectFilter{
		MemberUserID: &actor, Status: "ACTIVE", Query: "a' OR 1=1 --", Page: 2, PageSize: 10,
	})
	if err != nil || len(queries) != 2 {
		t.Fatalf("queries=%v error=%v", queries, err)
	}
	for _, query := range queries {
		if !strings.Contains(query.sql, "(pm_user_id =") || !strings.Contains(query.sql, "OR pc_user_id =") ||
			!strings.Contains(query.sql, "status =") || !strings.Contains(query.sql, "project_code ILIKE") || strings.Contains(query.sql, "a' OR 1=1") {
			t.Fatalf("missing bound filters: %s", query.sql)
		}
		searchBound := false
		for _, arg := range query.args {
			if arg == "%a' OR 1=1 --%" {
				searchBound = true
			}
		}
		if !searchBound {
			t.Fatalf("search not bound: %v", query.args)
		}
	}
	if !strings.Contains(queries[0].sql, "count(*)") || strings.Contains(queries[0].sql, "LIMIT") || strings.Contains(queries[0].sql, "OFFSET") {
		t.Fatalf("count query=%s", queries[0].sql)
	}
	if strings.Contains(queries[1].sql, "count(*)") || !strings.Contains(queries[1].sql, "ORDER BY id DESC") || !strings.Contains(queries[1].sql, "LIMIT") || !strings.Contains(queries[1].sql, "OFFSET") {
		t.Fatalf("page query=%s", queries[1].sql)
	}
}
