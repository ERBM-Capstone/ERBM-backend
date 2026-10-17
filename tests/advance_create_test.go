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
	appRouter "github.com/ERBM-Capstone/ERBM-backend/internal/router"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestAdvanceCreateRouteRequiresAuthenticationAndPC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := jwtmanager.NewManager("advance-create-access-key-with-at-least-32-bytes", "advance-create-refresh-key-with-at-least-32-bytes", "advance-create", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r := appRouter.New(nil, tokens, false, service.LoginPolicy{}, service.ActivationConfig{}, nil)
	for _, role := range []string{"", "ADMIN", "PM", "ACCOUNTING", "EMPLOYEE"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/advances", strings.NewReader(`{"project_id":1,"purpose":"Travel","amount":"100.00"}`))
		req.Header.Set("Content-Type", "application/json")
		want := http.StatusUnauthorized
		if role != "" {
			token, err := tokens.GenerateAccessToken(25, role)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			want = http.StatusForbidden
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("role %q: status=%d, want=%d", role, w.Code, want)
		}
	}
}

// Database operations are substituted here; actual transaction/locking behavior
// is covered by the opt-in PostgreSQL tests.
type advanceCreateRepoStub struct {
	project                                model.Project
	member                                 bool
	number                                 int64
	created                                model.Advance
	events                                 []model.AdvanceWorkflowEvent
	projectErr, eventErr, createErr, txErr error
	txCalls                                int
}

func newAdvanceCreateRepoStub() *advanceCreateRepoStub {
	return &advanceCreateRepoStub{project: model.Project{ID: 10, ProjectCode: "PRJ001", Name: "Project A", Status: model.ProjectStatusActive}, member: true}
}

func (r *advanceCreateRepoStub) List(context.Context, model.AdvanceFilter) ([]model.AdvanceListItem, int64, error) {
	return nil, 0, errors.New("unexpected list")
}
func (r *advanceCreateRepoStub) GetProjectAccess(context.Context, int64, int64) (*model.AdvanceProjectAccess, error) {
	return nil, errors.New("unexpected access")
}
func (r *advanceCreateRepoStub) WithTx(ctx context.Context, fn func(*gorm.DB) error) error {
	r.txCalls++
	if r.txErr != nil {
		return r.txErr
	}
	return fn(&gorm.DB{})
}
func (r *advanceCreateRepoStub) GetProjectForUpdate(context.Context, *gorm.DB, int64) (*model.Project, error) {
	return &r.project, r.projectErr
}
func (r *advanceCreateRepoStub) IsCoordinator(context.Context, *gorm.DB, int64, int64) (bool, error) {
	return r.member, nil
}
func (r *advanceCreateRepoStub) NextAdvanceNumber(context.Context, *gorm.DB, int64) (int64, error) {
	r.number++
	return r.number, nil
}
func (r *advanceCreateRepoStub) Create(_ context.Context, _ *gorm.DB, advance *model.Advance) error {
	if r.createErr != nil {
		return r.createErr
	}
	advance.ID = 7
	advance.CreatedAt = time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)
	advance.UpdatedAt = advance.CreatedAt
	r.created = *advance
	return nil
}
func (r *advanceCreateRepoStub) RecordEvent(_ context.Context, _ *gorm.DB, event *model.AdvanceWorkflowEvent) error {
	if r.eventErr != nil {
		return r.eventErr
	}
	r.events = append(r.events, *event)
	return nil
}
func (r *advanceCreateRepoStub) GetListItem(context.Context, *gorm.DB, int64) (*model.AdvanceListItem, error) {
	return &model.AdvanceListItem{ID: r.created.ID, AdvanceCode: r.created.AdvanceCode,
		ProjectID: r.created.ProjectID, ProjectCode: r.project.ProjectCode, ProjectName: r.project.Name,
		Purpose: r.created.Purpose, Amount: r.created.Amount, Status: r.created.Status,
		CreatedBy: r.created.CreatedBy, RequesterFirstName: "Somchai", RequesterLastName: "Jaidee",
		CreatedAt: r.created.CreatedAt, UpdatedAt: r.created.UpdatedAt}, nil
}

func TestAdvanceCreateUsesAuthenticatedCreatorAndExactPositiveMoney(t *testing.T) {
	for _, tt := range []struct{ amount, want string }{
		{"15000", "15000.00"}, {"00001.5", "1.50"}, {"0.01", "0.01"}, {"9999999999999.99", "9999999999999.99"},
	} {
		t.Run(tt.amount, func(t *testing.T) {
			repo := newAdvanceCreateRepoStub()
			item, err := service.NewAdvanceService(repo).Create(context.Background(), 25, "PC", &dto.CreateAdvanceRequest{ProjectID: 10, Purpose: " Travel ", Amount: tt.amount})
			if err != nil {
				t.Fatal(err)
			}
			if item.AdvanceCode != "ADV-PRJ001-001" || item.Amount != tt.want || item.Purpose != "Travel" || item.Status != model.AdvanceStatusDraft || item.CreatedBy != 25 || repo.created.UpdatedBy != 25 || item.SubmittedAt != nil {
				t.Fatalf("created item = %#v; persisted=%#v", item, repo.created)
			}
			if len(repo.events) != 1 {
				t.Fatalf("creation must include an audit event: %#v", repo.events)
			}
			event := repo.events[0]
			var snapshot map[string]any
			if err := json.Unmarshal([]byte(event.AfterData), &snapshot); err != nil {
				t.Fatal(err)
			}
			if event.AdvanceID != 7 || event.ActorUserID != 25 || event.Action != "CREATE" || event.PreviousStatus != nil || event.ResultingStatus != model.AdvanceStatusDraft || snapshot["amount"] != tt.want || snapshot["advance_code"] != "ADV-PRJ001-001" {
				t.Fatalf("event=%#v; snapshot=%#v", event, snapshot)
			}
		})
	}
	// The width is a minimum: code 1000 must not be truncated to 100.
	repo := newAdvanceCreateRepoStub()
	repo.number = 999
	item, err := service.NewAdvanceService(repo).Create(context.Background(), 25, "PC", &dto.CreateAdvanceRequest{ProjectID: 10, Purpose: "Travel", Amount: "1"})
	if err != nil || item.AdvanceCode != "ADV-PRJ001-1000" {
		t.Fatalf("code beyond 999 = %#v, %v", item, err)
	}
}

func TestAdvanceCreateRejectsMissingInvalidAndNonpositiveInputBeforeWriting(t *testing.T) {
	for _, req := range []*dto.CreateAdvanceRequest{
		nil, {}, {ProjectID: -1, Purpose: "Travel", Amount: "1"}, {ProjectID: 10, Purpose: "  ", Amount: "1"},
		{ProjectID: 10, Purpose: "Travel", Amount: ""}, {ProjectID: 10, Purpose: "Travel", Amount: "0"},
		{ProjectID: 10, Purpose: "Travel", Amount: "0.00"}, {ProjectID: 10, Purpose: "Travel", Amount: "-1.00"},
		{ProjectID: 10, Purpose: "Travel", Amount: "1.001"}, {ProjectID: 10, Purpose: "Travel", Amount: "10000000000000"},
		{ProjectID: 10, Purpose: "Travel", Amount: "1e3"}, {ProjectID: 10, Purpose: "Travel", Amount: "NaN"},
	} {
		repo := newAdvanceCreateRepoStub()
		_, err := service.NewAdvanceService(repo).Create(context.Background(), 25, "PC", req)
		if !errors.Is(err, dto.ErrBadRequest) || repo.txCalls != 0 {
			t.Fatalf("request=%#v: err=%v, transactions=%d", req, err, repo.txCalls)
		}
	}
}

func TestAdvanceCreateEnforcesActorProjectAndAtomicAudit(t *testing.T) {
	for _, tt := range []struct {
		name      string
		actor     int64
		role      string
		configure func(*advanceCreateRepoStub)
		want      error
	}{
		{"anonymous", 0, "PC", nil, dto.ErrUnauthorized},
		{"admin cannot create", 25, "ADMIN", nil, dto.ErrForbidden},
		{"unassigned PC", 25, "PC", func(r *advanceCreateRepoStub) { r.member = false }, dto.ErrForbidden},
		{"missing project", 25, "PC", func(r *advanceCreateRepoStub) { r.projectErr = dto.ErrProjectNotFound }, dto.ErrProjectNotFound},
		{"closed project", 25, "PC", func(r *advanceCreateRepoStub) { r.project.Status = model.ProjectStatusClosed }, dto.ErrProjectClosed},
		{"event failure", 25, "PC", func(r *advanceCreateRepoStub) { r.eventErr = errors.New("database detail") }, dto.ErrInternalServer},
		{"insert failure", 25, "PC", func(r *advanceCreateRepoStub) { r.createErr = errors.New("database detail") }, dto.ErrInternalServer},
		{"transaction failure", 25, "PC", func(r *advanceCreateRepoStub) { r.txErr = errors.New("database detail") }, dto.ErrInternalServer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newAdvanceCreateRepoStub()
			if tt.configure != nil {
				tt.configure(repo)
			}
			item, err := service.NewAdvanceService(repo).Create(context.Background(), tt.actor, tt.role, &dto.CreateAdvanceRequest{ProjectID: 10, Purpose: "Travel", Amount: "1"})
			if !errors.Is(err, tt.want) || item != nil {
				t.Fatalf("item=%#v, error=%v", item, err)
			}
			if tt.want != dto.ErrInternalServer && (repo.number != 0 || repo.created.ID != 0 || len(repo.events) != 0) {
				t.Fatal("denied requests must not allocate a number or write records")
			}
		})
	}
}

func advanceCreateHTTP(repo *advanceCreateRepoStub, body string, authenticated bool) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ContextKeyUserID, int64(25))
			c.Set(middleware.ContextKeyRole, "PC")
			c.Next()
		})
	}
	r.POST("/api/v1/advances", handler.NewAdvanceHandler(service.NewAdvanceService(repo)).Create)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/advances", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAdvanceCreateHTTPContractIgnoresForgedWorkflowAndActor(t *testing.T) {
	repo := newAdvanceCreateRepoStub()
	w := advanceCreateHTTP(repo, `{"project_id":10,"purpose":"Travel","amount":"15000.00","created_by":999,"updated_by":999,"requester":{"user_id":999},"status":"ACTIVE","advance_code":"forged"}`, true)
	var response struct {
		Success bool                        `json:"success"`
		Data    dto.AdvanceListItemResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusCreated || !response.Success || response.Data.AdvanceCode != "ADV-PRJ001-001" || response.Data.Requester.UserID != 25 || response.Data.Requester.FirstName != "Somchai" || response.Data.Requester.LastName != "Jaidee" || response.Data.Amount != "15000.00" || response.Data.Status != model.AdvanceStatusDraft || response.Data.CreatedAt != "2026-10-16T03:00:00Z" || strings.Contains(w.Body.String(), "currency_code") {
		t.Fatalf("response=%d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{`{}`, `null`, `{"project_id":10,"purpose":"Travel"}`, `{"project_id":10,"purpose":"Travel","amount":0}`, `{"project_id":10,"purpose":"Travel","amount":null}`, `{"project_id":10,"purpose":"Travel","amount":"0"}`} {
		r := newAdvanceCreateRepoStub()
		result := advanceCreateHTTP(r, body, true)
		if result.Code != http.StatusBadRequest || r.txCalls != 0 {
			t.Fatalf("body=%s: %d %s", body, result.Code, result.Body.String())
		}
	}
	for _, tt := range []struct {
		configure     func(*advanceCreateRepoStub)
		authenticated bool
		status        int
	}{
		{func(*advanceCreateRepoStub) {}, false, 401}, {func(r *advanceCreateRepoStub) { r.member = false }, true, 403},
		{func(r *advanceCreateRepoStub) { r.projectErr = dto.ErrProjectNotFound }, true, 404}, {func(r *advanceCreateRepoStub) { r.project.Status = model.ProjectStatusClosed }, true, 409},
		{func(r *advanceCreateRepoStub) { r.eventErr = errors.New("private SQL detail") }, true, 500},
	} {
		r := newAdvanceCreateRepoStub()
		tt.configure(r)
		result := advanceCreateHTTP(r, `{"project_id":10,"purpose":"Travel","amount":"1"}`, tt.authenticated)
		if result.Code != tt.status || strings.Contains(result.Body.String(), "private SQL detail") {
			t.Fatalf("response=%d %s", result.Code, result.Body.String())
		}
	}
}
