package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	appRouter "github.com/ERBM-Capstone/ERBM-backend/internal/router"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type advanceListRepoStub struct {
	*repository.AdvanceRepository // Unexpected write methods must never be used by List.
	items                         []model.AdvanceListItem
	total                         int64
	listError                     error
	accessError                   error
	access                        model.AdvanceProjectAccess
	filter                        model.AdvanceFilter
	listCalls                     int
	accessUserID                  int64
}

func (r *advanceListRepoStub) List(_ context.Context, filter model.AdvanceFilter) ([]model.AdvanceListItem, int64, error) {
	r.filter = filter
	r.listCalls++
	return r.items, r.total, r.listError
}

func (r *advanceListRepoStub) GetProjectAccess(_ context.Context, _ int64, userID int64) (*model.AdvanceProjectAccess, error) {
	r.accessUserID = userID
	if r.accessError != nil {
		return nil, r.accessError
	}
	return &r.access, nil
}

func TestAdvanceListServiceUsesTrustedActorAndFilteredTotal(t *testing.T) {
	for _, role := range []string{"PC", "PM", "ADMIN", "ACCOUNTING"} {
		t.Run(role, func(t *testing.T) {
			repo := &advanceListRepoStub{
				items: []model.AdvanceListItem{{ID: 1, Amount: "15000.00"}}, total: 3,
			}
			svc := service.NewAdvanceService(repo)
			items, total, err := svc.List(context.Background(), 25, role, model.AdvanceFilter{
				Page: 1, PageSize: 20, Query: " travel ",
				ActorUserID: 999, ActorRole: "EMPLOYEE",
			})
			if err != nil || total != 3 || len(items) != 1 || items[0].Amount != "15000.00" {
				t.Fatalf("List = %#v, %d, %v", items, total, err)
			}
			if repo.filter.ActorUserID != 25 || repo.filter.ActorRole != role || repo.filter.Query != "travel" {
				t.Fatalf("repository scope = %#v", repo.filter)
			}
		})
	}
}

func TestAdvanceListServiceChecksExplicitProjectAccess(t *testing.T) {
	projectID := int64(10)
	for _, tt := range []struct {
		name   string
		role   string
		access model.AdvanceProjectAccess
		want   error
	}{
		{"coordinator", "PC", model.AdvanceProjectAccess{ProjectID: 10, IsCoordinator: true}, nil},
		{"other coordinator", "PC", model.AdvanceProjectAccess{ProjectID: 10, PMUserID: 25}, dto.ErrForbidden},
		{"assigned manager", "PM", model.AdvanceProjectAccess{ProjectID: 10, PMUserID: 25}, nil},
		{"other manager", "PM", model.AdvanceProjectAccess{ProjectID: 10, PMUserID: 99, IsCoordinator: true}, dto.ErrForbidden},
		{"administrator", "ADMIN", model.AdvanceProjectAccess{ProjectID: 10}, nil},
		{"accounting", "ACCOUNTING", model.AdvanceProjectAccess{ProjectID: 10}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &advanceListRepoStub{access: tt.access}
			_, _, err := service.NewAdvanceService(repo).List(context.Background(), 25, tt.role,
				model.AdvanceFilter{ProjectID: &projectID, Page: 1, PageSize: 20})
			if !errors.Is(err, tt.want) || repo.accessUserID != 25 {
				t.Fatalf("error = %v; access actor = %d", err, repo.accessUserID)
			}
			if tt.want != nil && repo.listCalls != 0 {
				t.Fatal("an inaccessible Project must not query Advance records")
			}
		})
	}
	for _, tt := range []struct{ cause, want error }{
		{dto.ErrProjectNotFound, dto.ErrProjectNotFound},
		{errors.New("database failure"), dto.ErrInternalServer},
	} {
		repo := &advanceListRepoStub{accessError: tt.cause}
		_, _, err := service.NewAdvanceService(repo).List(context.Background(), 25, "PC",
			model.AdvanceFilter{ProjectID: &projectID, Page: 1, PageSize: 20})
		if !errors.Is(err, tt.want) || repo.listCalls != 0 {
			t.Fatalf("project lookup failure = %v; list calls = %d", err, repo.listCalls)
		}
	}
}

func TestAdvanceListServiceRejectsInvalidScopeAndFilters(t *testing.T) {
	zeroProject := int64(0)
	maxInt := int(^uint(0) >> 1)
	for _, tt := range []struct {
		name   string
		userID int64
		role   string
		filter model.AdvanceFilter
		want   error
	}{
		{"no actor", 0, "PC", model.AdvanceFilter{Page: 1, PageSize: 20}, dto.ErrUnauthorized},
		{"employee", 25, "EMPLOYEE", model.AdvanceFilter{Page: 1, PageSize: 20}, dto.ErrForbidden},
		{"invalid project", 25, "PC", model.AdvanceFilter{ProjectID: &zeroProject, Page: 1, PageSize: 20}, dto.ErrBadRequest},
		{"invalid status", 25, "PC", model.AdvanceFilter{Status: "REJECTED", Page: 1, PageSize: 20}, dto.ErrBadRequest},
		{"page zero", 25, "PC", model.AdvanceFilter{Page: 0, PageSize: 20}, dto.ErrBadRequest},
		{"oversized page", 25, "PC", model.AdvanceFilter{Page: 1, PageSize: 101}, dto.ErrBadRequest},
		{"offset overflow", 25, "PC", model.AdvanceFilter{Page: maxInt, PageSize: 100}, dto.ErrBadRequest},
		{"long query", 25, "PC", model.AdvanceFilter{Query: strings.Repeat("ก", 256), Page: 1, PageSize: 20}, dto.ErrBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &advanceListRepoStub{}
			_, _, err := service.NewAdvanceService(repo).List(context.Background(), tt.userID, tt.role, tt.filter)
			if !errors.Is(err, tt.want) || repo.listCalls != 0 {
				t.Fatalf("error = %v; list calls = %d", err, repo.listCalls)
			}
		})
	}
	repo := &advanceListRepoStub{listError: errors.New("unexpected database failure")}
	_, _, err := service.NewAdvanceService(repo).List(context.Background(), 25, "PC", model.AdvanceFilter{Page: 1, PageSize: 20})
	if !errors.Is(err, dto.ErrInternalServer) {
		t.Fatalf("persistence failure = %v", err)
	}
}

type advanceListServiceStub struct {
	*service.AdvanceService // Detail/update methods are unused by List.
	items                   []model.AdvanceListItem
	total                   int64
	err                     error
	actor                   int64
	role                    string
	filter                  model.AdvanceFilter
	calls                   int
}

func (s *advanceListServiceStub) List(_ context.Context, userID int64, role string, filter model.AdvanceFilter) ([]model.AdvanceListItem, int64, error) {
	s.actor, s.role, s.filter = userID, role, filter
	s.calls++
	return s.items, s.total, s.err
}

func (*advanceListServiceStub) Create(context.Context, int64, string, *dto.CreateAdvanceRequest) (*model.AdvanceListItem, error) {
	return nil, dto.ErrInternalServer
}

func advanceListRequest(h http.Handler, url string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, url, nil))
	return recorder
}

func advanceListTestRouter(svc *advanceListServiceStub, authenticated bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ContextKeyUserID, int64(25))
			c.Set(middleware.ContextKeyRole, "PC")
			c.Next()
		})
	}
	r.GET("/api/v1/advances", handler.NewAdvanceHandler(svc).List)
	return r
}

func TestAdvanceListHandlerContractAndTrustedActor(t *testing.T) {
	when := time.Date(2026, 10, 15, 2, 0, 0, 0, time.UTC)
	svc := &advanceListServiceStub{total: 3, items: []model.AdvanceListItem{{
		ID: 7, AdvanceCode: "ADV-PRJ001-001", RequesterFirstName: "Somchai", RequesterLastName: "Jaidee", ProjectID: 10, ProjectCode: "PRJ-001", ProjectName: "Project A",
		Purpose: "Travel", Amount: "15000.00", Status: model.AdvanceStatusDraft,
		CreatedBy: 25, CreatedAt: when, UpdatedAt: when,
	}}}
	r := advanceListTestRouter(svc, true)
	w := advanceListRequest(r, "/api/v1/advances?project_id=10&status=DRAFT&q=Travel&actor_user_id=999&actor_role=ADMIN")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	if svc.actor != 25 || svc.role != "PC" || svc.filter.Page != 1 || svc.filter.PageSize != 20 || svc.filter.ProjectID == nil || *svc.filter.ProjectID != 10 {
		t.Fatalf("actor/filter = %d, %s, %#v", svc.actor, svc.role, svc.filter)
	}
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID      int64 `json:"id"`
			Project struct {
				ID   int64  `json:"id"`
				Code string `json:"project_code"`
				Name string `json:"name"`
			} `json:"project"`
			Amount      string  `json:"amount"`
			CreatedBy   int64   `json:"created_by"`
			SubmittedAt *string `json:"submitted_at"`
			CreatedAt   string  `json:"created_at"`
		} `json:"data"`
		Meta struct {
			Page    int   `json:"page"`
			PerPage int   `json:"per_page"`
			Total   int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Success || len(body.Data) != 1 || body.Data[0].Amount != "15000.00" || body.Data[0].Project.ID != 10 || body.Data[0].Project.Code != "PRJ-001" || body.Data[0].Project.Name != "Project A" || body.Data[0].CreatedBy != 25 || body.Data[0].SubmittedAt != nil || body.Data[0].CreatedAt != "2026-10-15T02:00:00Z" || body.Meta.Total != 3 || body.Meta.Page != 1 || body.Meta.PerPage != 20 {
		t.Fatalf("response = %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "currency_code") {
		t.Fatal("THB-only response must not include currency_code")
	}
	var display struct {
		Data []dto.AdvanceListItemResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &display); err != nil {
		t.Fatal(err)
	}
	if display.Data[0].AdvanceCode != "ADV-PRJ001-001" || display.Data[0].Requester.UserID != 25 || display.Data[0].Requester.FirstName != "Somchai" || display.Data[0].Requester.LastName != "Jaidee" {
		t.Fatalf("list display fields = %s", w.Body.String())
	}
}

func TestAdvanceListHandlerEmptyResultAndErrors(t *testing.T) {
	svc := &advanceListServiceStub{}
	w := advanceListRequest(advanceListTestRouter(svc, true), "/api/v1/advances?page=2&page_size=10")
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || string(body["data"]) != "[]" || string(body["meta"]) != `{"page":2,"per_page":10,"total":0}` {
		t.Fatalf("empty response = %d %s", w.Code, w.Body.String())
	}
	defaults := &advanceListServiceStub{}
	w = advanceListRequest(advanceListTestRouter(defaults, true), "/api/v1/advances?page=0&page_size=0")
	if w.Code != http.StatusOK || defaults.filter.Page != 1 || defaults.filter.PageSize != 20 {
		t.Fatalf("zero paging must retain current defaults: %d, %#v", w.Code, defaults.filter)
	}
	for _, query := range []string{"page=-1", "page_size=101", "project_id=0", "project_id=-1", "project_id=abc", "status=REJECTED", "q=" + strings.Repeat("a", 256)} {
		svc := &advanceListServiceStub{}
		w := advanceListRequest(advanceListTestRouter(svc, true), "/api/v1/advances?"+query)
		if w.Code != http.StatusBadRequest || svc.calls != 0 {
			t.Fatalf("query %q: status=%d; calls=%d", query, w.Code, svc.calls)
		}
	}
	w = advanceListRequest(advanceListTestRouter(&advanceListServiceStub{}, false), "/api/v1/advances")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", w.Code)
	}
	for _, tt := range []struct {
		err    error
		status int
	}{
		{dto.ErrForbidden, http.StatusForbidden},
		{dto.ErrProjectNotFound, http.StatusNotFound},
		{dto.ErrInternalServer, http.StatusInternalServerError},
	} {
		w := advanceListRequest(advanceListTestRouter(&advanceListServiceStub{err: tt.err}, true), "/api/v1/advances")
		if w.Code != tt.status {
			t.Fatalf("error %v: status = %d", tt.err, w.Code)
		}
	}
}

func TestAdvanceListRouteRequiresAuthenticationAndAllowedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := jwtmanager.NewManager("advance-test-access-key-with-at-least-32-bytes", "advance-test-refresh-key-with-at-least-32-bytes", "advance-test", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r := appRouter.New(nil, tokens, false, service.LoginPolicy{}, service.ActivationConfig{}, nil)
	w := advanceListRequest(r, "/api/v1/advances")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("registered protected route status = %d", w.Code)
	}
	employeeToken, err := tokens.GenerateAccessToken(25, "EMPLOYEE")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/advances", nil)
	req.Header.Set("Authorization", "Bearer "+employeeToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("employee route status = %d", w.Code)
	}
}
