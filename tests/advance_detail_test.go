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
	"gorm.io/gorm"
)

func TestAdvanceDetailRoutesEnforceAuthenticationRolesAndURI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := jwtmanager.NewManager("advance-detail-access-key-with-at-least-32-bytes", "advance-detail-refresh-key-with-at-least-32-bytes", "advance-detail", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r := appRouter.New(nil, tokens, false, service.LoginPolicy{}, service.ActivationConfig{}, nil)
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		for _, role := range []string{"", "EMPLOYEE", "ADMIN", "ACCOUNTING", "PM", "PC"} {
			t.Run(method+"/"+role, func(t *testing.T) {
				req := httptest.NewRequest(method, "/api/v1/advances/invalid", strings.NewReader(`{"purpose":"Travel"}`))
				req.Header.Set("Content-Type", "application/json")
				want := http.StatusBadRequest
				if role == "" {
					want = http.StatusUnauthorized
				} else {
					token, err := tokens.GenerateAccessToken(25, role)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", "Bearer "+token)
					if role == "EMPLOYEE" || (method == http.MethodPatch && role != "PC") {
						want = http.StatusForbidden
					}
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != want {
					t.Fatalf("status=%d, want=%d: %s", w.Code, want, w.Body.String())
				}
			})
		}
	}
}

type advanceDetailRepoStub struct {
	*repository.AdvanceRepository
	advance                                                                        model.Advance
	project                                                                        model.Project
	access                                                                         model.AdvanceProjectAccess
	events                                                                         []model.AdvanceWorkflowEvent
	calls                                                                          []string
	lookupErr, accessErr, projectErr, lockErr, updateErr, eventErr, readErr, txErr error
}

func advanceDetailFixture() *advanceDetailRepoStub {
	pc := int64(25)
	when := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	return &advanceDetailRepoStub{
		advance: model.Advance{ID: 7, ProjectID: 10, AdvanceCode: "ADV-PRJ001-001", Purpose: "Travel", Amount: "15000.00", Status: model.AdvanceStatusDraft, CreatedBy: 25, UpdatedBy: 25, CreatedAt: when, UpdatedAt: when},
		project: model.Project{ID: 10, ProjectCode: "PRJ001", Name: "Project A", PCUserID: &pc, PMUserID: 30, Status: model.ProjectStatusActive},
		access:  model.AdvanceProjectAccess{ProjectID: 10, PMUserID: 30, IsCoordinator: true},
	}
}

func (r *advanceDetailRepoStub) item() *model.AdvanceListItem {
	a := r.advance
	return &model.AdvanceListItem{ID: a.ID, AdvanceCode: a.AdvanceCode, ProjectID: a.ProjectID, ProjectCode: r.project.ProjectCode, ProjectName: r.project.Name, PMUserID: r.project.PMUserID, PCUserID: r.project.PCUserID, Purpose: a.Purpose, Amount: a.Amount, Status: a.Status, CreatedBy: a.CreatedBy, RequesterFirstName: "Somchai", RequesterLastName: "Jaidee", SubmittedAt: a.SubmittedAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
}

func (r *advanceDetailRepoStub) GetByID(_ context.Context, id int64) (*model.AdvanceListItem, error) {
	r.calls = append(r.calls, "lookup")
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	if id != 7 {
		return nil, dto.ErrAdvanceNotFound
	}
	return r.item(), nil
}

func (r *advanceDetailRepoStub) GetProjectAccess(context.Context, int64, int64) (*model.AdvanceProjectAccess, error) {
	return &r.access, r.accessErr
}

func (r *advanceDetailRepoStub) WithTx(_ context.Context, fn func(*gorm.DB) error) error {
	r.calls = append(r.calls, "transaction")
	if r.txErr != nil {
		return r.txErr
	}
	return fn(&gorm.DB{})
}

func (r *advanceDetailRepoStub) GetProjectForUpdate(_ context.Context, _ *gorm.DB, id int64) (*model.Project, error) {
	r.calls = append(r.calls, "lock project")
	if id != 10 {
		return nil, errors.New("incorrect project")
	}
	return &r.project, r.projectErr
}

func (r *advanceDetailRepoStub) GetByIDForUpdate(_ context.Context, _ *gorm.DB, id int64) (*model.Advance, error) {
	r.calls = append(r.calls, "lock advance")
	if id != 7 {
		return nil, dto.ErrAdvanceNotFound
	}
	copy := r.advance
	return &copy, r.lockErr
}

func (r *advanceDetailRepoStub) Update(_ context.Context, _ *gorm.DB, a *model.Advance) error {
	r.calls = append(r.calls, "update")
	if r.updateErr != nil {
		return r.updateErr
	}
	r.advance = *a
	return nil
}

func (r *advanceDetailRepoStub) RecordEvent(_ context.Context, _ *gorm.DB, event *model.AdvanceWorkflowEvent) error {
	r.calls = append(r.calls, "audit")
	if r.eventErr != nil {
		return r.eventErr
	}
	r.events = append(r.events, *event)
	return nil
}

func (r *advanceDetailRepoStub) GetListItem(context.Context, *gorm.DB, int64) (*model.AdvanceListItem, error) {
	r.calls = append(r.calls, "response")
	return r.item(), r.readErr
}

func TestAdvanceGetServiceEnforcesProjectAccessAndPrivateDraft(t *testing.T) {
	for _, tt := range []struct {
		name, role string
		actor      int64
		status     model.AdvanceStatus
		member     bool
		want       error
	}{
		{"own draft", "PC", 25, model.AdvanceStatusDraft, true, nil},
		{"former coordinator", "PC", 25, model.AdvanceStatusDraft, false, dto.ErrForbidden},
		{"other PC draft", "PC", 26, model.AdvanceStatusDraft, true, dto.ErrForbidden},
		{"manager draft", "PM", 30, model.AdvanceStatusDraft, true, dto.ErrForbidden},
		{"admin draft", "ADMIN", 40, model.AdvanceStatusDraft, true, dto.ErrForbidden},
		{"accounting draft", "ACCOUNTING", 40, model.AdvanceStatusDraft, true, dto.ErrForbidden},
		{"manager pending", "PM", 30, model.AdvanceStatusPendingPMApproval, true, nil},
		{"other manager", "PM", 31, model.AdvanceStatusPendingPMApproval, true, dto.ErrForbidden},
		{"coordinator active", "PC", 25, model.AdvanceStatusActive, true, nil},
		{"admin closed", "ADMIN", 40, model.AdvanceStatusClosed, false, nil},
		{"accounting approved", "ACCOUNTING", 40, model.AdvanceStatusApproved, false, nil},
		{"anonymous", "PC", 0, model.AdvanceStatusDraft, true, dto.ErrUnauthorized},
		{"unsupported role", "EMPLOYEE", 25, model.AdvanceStatusDraft, true, dto.ErrForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := advanceDetailFixture()
			r.advance.Status, r.access.IsCoordinator = tt.status, tt.member
			if !tt.member {
				r.project.PCUserID = nil
			}
			item, err := service.NewAdvanceService(r).GetByID(context.Background(), tt.actor, tt.role, 7)
			if !errors.Is(err, tt.want) {
				t.Fatalf("item=%#v, err=%v, want=%v", item, err, tt.want)
			}
			if tt.want == nil && (item == nil || item.ID != 7 || item.Amount != "15000.00") {
				t.Fatalf("item=%#v", item)
			}
			if tt.want != nil && item != nil {
				t.Fatal("denied reads must not return data")
			}
		})
	}
}

func TestAdvanceGetServiceMapsErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		id        int64
		configure func(*advanceDetailRepoStub)
		want      error
	}{
		{"invalid id", 0, nil, dto.ErrBadRequest},
		{"missing", 99, nil, dto.ErrAdvanceNotFound},
		{"lookup failure", 7, func(r *advanceDetailRepoStub) { r.lookupErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"wrapped not found", 7, func(r *advanceDetailRepoStub) {
			r.lookupErr = errors.Join(dto.ErrAdvanceNotFound, errors.New("database secret"))
		}, dto.ErrAdvanceNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := advanceDetailFixture()
			if tt.configure != nil {
				tt.configure(r)
			}
			item, err := service.NewAdvanceService(r).GetByID(context.Background(), 25, "PC", tt.id)
			if item != nil || !errors.Is(err, tt.want) {
				t.Fatalf("item=%#v, err=%v", item, err)
			}
		})
	}
}

func TestAdvanceGetDoesNotCombineOldDataWithNewProjectPermissions(t *testing.T) {
	r := advanceDetailFixture()
	r.advance.Status = model.AdvanceStatusReturnedForRevision
	// At the time the Advance is read, actor 30 is not its manager. A later
	// Project-access query could observe reassignment to actor 30 after a PC
	// has edited the returned request back to a private DRAFT.
	r.project.PMUserID = 31
	r.access.PMUserID = 30
	item, err := service.NewAdvanceService(r).GetByID(context.Background(), 30, "PM", 7)
	if item != nil || !errors.Is(err, dto.ErrForbidden) {
		t.Fatalf("read combined different visibility snapshots: item=%#v err=%v", item, err)
	}
}

func TestAdvancePatchPreservesOmittedFieldsAndWritesExactAudit(t *testing.T) {
	for _, tt := range []struct {
		name, body, purpose, amount string
		status                      model.AdvanceStatus
	}{
		{"purpose only", `{"purpose":"  Travel and hotel  "}`, "Travel and hotel", "15000.00", model.AdvanceStatusDraft},
		{"amount only", `{"amount":"00001.5"}`, "Travel", "1.50", model.AdvanceStatusDraft},
		{"null retains amount", `{"amount":null,"purpose":"Updated"}`, "Updated", "15000.00", model.AdvanceStatusDraft},
		{"null retains purpose", `{"purpose":null,"amount":"1"}`, "Travel", "1.00", model.AdvanceStatusDraft},
		{"revision", `{"amount":"9999999999999.99"}`, "Travel", "9999999999999.99", model.AdvanceStatusReturnedForRevision},
		{"untrusted fields", `{"purpose":"Updated","project_id":999,"advance_code":"FORGED","created_by":999,"updated_by":999,"status":"ACTIVE"}`, "Updated", "15000.00", model.AdvanceStatusDraft},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := advanceDetailFixture()
			r.advance.Status = tt.status
			var req dto.UpdateAdvanceRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatal(err)
			}
			item, err := service.NewAdvanceService(r).Update(context.Background(), 25, "PC", 7, &req)
			if err != nil {
				t.Fatal(err)
			}
			if item.Amount != tt.amount || item.Purpose != tt.purpose || item.Status != model.AdvanceStatusDraft || item.ProjectID != 10 || item.AdvanceCode != "ADV-PRJ001-001" || item.CreatedBy != 25 || r.advance.UpdatedBy != 25 {
				t.Fatalf("item=%#v persisted=%#v", item, r.advance)
			}
			if strings.Join(r.calls, ",") != "lookup,transaction,lock project,lock advance,update,audit,response" {
				t.Fatalf("transaction order=%v", r.calls)
			}
			if len(r.events) != 1 {
				t.Fatalf("events=%v", r.events)
			}
			e := r.events[0]
			if e.BeforeData == nil {
				t.Fatal("missing before snapshot")
			}
			var before, after model.Advance
			if err := json.Unmarshal([]byte(*e.BeforeData), &before); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(e.AfterData), &after); err != nil {
				t.Fatal(err)
			}
			if before.Amount != "15000.00" || before.Purpose != "Travel" || before.Status != tt.status || after.Amount != tt.amount || after.Purpose != tt.purpose || after.Status != model.AdvanceStatusDraft || e.AdvanceID != 7 || e.ActorUserID != 25 || e.Action != "UPDATE" || e.PreviousStatus == nil || *e.PreviousStatus != tt.status || e.ResultingStatus != model.AdvanceStatusDraft || !e.OccurredAt.Equal(r.advance.UpdatedAt) || !after.UpdatedAt.After(before.UpdatedAt) {
				t.Fatalf("before=%#v after=%#v event=%#v", before, after, e)
			}
		})
	}
}

func TestAdvancePatchRejectsUnauthorizedStateAndPersistenceErrors(t *testing.T) {
	for _, tt := range []struct {
		name, role string
		actor      int64
		configure  func(*advanceDetailRepoStub)
		want       error
	}{
		{"anonymous", "PC", 0, nil, dto.ErrUnauthorized},
		{"admin", "ADMIN", 25, nil, dto.ErrForbidden},
		{"PM", "PM", 25, nil, dto.ErrForbidden},
		{"accounting", "ACCOUNTING", 25, nil, dto.ErrForbidden},
		{"different creator", "PC", 25, func(r *advanceDetailRepoStub) { r.advance.CreatedBy = 99 }, dto.ErrForbidden},
		{"former PC", "PC", 25, func(r *advanceDetailRepoStub) { pc := int64(99); r.project.PCUserID = &pc }, dto.ErrForbidden},
		{"unassigned project", "PC", 25, func(r *advanceDetailRepoStub) { r.project.PCUserID = nil }, dto.ErrForbidden},
		{"closed project", "PC", 25, func(r *advanceDetailRepoStub) { r.project.Status = model.ProjectStatusClosed }, dto.ErrProjectClosed},
		{"pending", "PC", 25, func(r *advanceDetailRepoStub) { r.advance.Status = model.AdvanceStatusPendingPMApproval }, dto.ErrAdvanceNotEditable},
		{"approved", "PC", 25, func(r *advanceDetailRepoStub) { r.advance.Status = model.AdvanceStatusApproved }, dto.ErrAdvanceNotEditable},
		{"active", "PC", 25, func(r *advanceDetailRepoStub) { r.advance.Status = model.AdvanceStatusActive }, dto.ErrAdvanceNotEditable},
		{"closed", "PC", 25, func(r *advanceDetailRepoStub) { r.advance.Status = model.AdvanceStatusClosed }, dto.ErrAdvanceNotEditable},
		{"missing", "PC", 25, func(r *advanceDetailRepoStub) { r.lookupErr = dto.ErrAdvanceNotFound }, dto.ErrAdvanceNotFound},
		{"lookup error", "PC", 25, func(r *advanceDetailRepoStub) { r.lookupErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"lock missing", "PC", 25, func(r *advanceDetailRepoStub) { r.lockErr = dto.ErrAdvanceNotFound }, dto.ErrAdvanceNotFound},
		{"project lock error", "PC", 25, func(r *advanceDetailRepoStub) { r.projectErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"advance lock error", "PC", 25, func(r *advanceDetailRepoStub) { r.lockErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"update error", "PC", 25, func(r *advanceDetailRepoStub) { r.updateErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"audit error", "PC", 25, func(r *advanceDetailRepoStub) { r.eventErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"response error", "PC", 25, func(r *advanceDetailRepoStub) { r.readErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"commit error", "PC", 25, func(r *advanceDetailRepoStub) { r.txErr = errors.New("database secret") }, dto.ErrInternalServer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := advanceDetailFixture()
			if tt.configure != nil {
				tt.configure(r)
			}
			purpose := "Updated"
			item, err := service.NewAdvanceService(r).Update(context.Background(), tt.actor, tt.role, 7, &dto.UpdateAdvanceRequest{Purpose: &purpose})
			if item != nil || !errors.Is(err, tt.want) {
				t.Fatalf("item=%#v err=%v, want=%v", item, err, tt.want)
			}
			if tt.want != dto.ErrInternalServer && (r.advance.Purpose != "Travel" || len(r.events) != 0) {
				t.Fatal("denied patch must not write")
			}
		})
	}
}

func advanceDetailHTTP(r *advanceDetailRepoStub, method, id, body string, authenticated bool) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if authenticated {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextKeyUserID, int64(25))
			c.Set(middleware.ContextKeyRole, "PC")
		})
	}
	h := handler.NewAdvanceHandler(service.NewAdvanceService(r))
	router.GET("/api/v1/advances/:id", h.GetByID)
	router.PATCH("/api/v1/advances/:id", h.Update)
	req := httptest.NewRequest(method, "/api/v1/advances/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAdvanceDetailHTTPValidationAndResponses(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
			r := advanceDetailFixture()
			w := advanceDetailHTTP(r, method, id, `{"purpose":"Updated"}`, true)
			if w.Code != http.StatusBadRequest || len(r.calls) != 0 {
				t.Fatalf("%s %s: %d %s", method, id, w.Code, w.Body.String())
			}
		}
		w := advanceDetailHTTP(advanceDetailFixture(), method, "7", `{"purpose":"Updated"}`, false)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s: %d", method, w.Code)
		}
		w = advanceDetailHTTP(advanceDetailFixture(), method, "99", `{"purpose":"Updated"}`, true)
		if w.Code != http.StatusNotFound {
			t.Fatalf("missing %s: %d", method, w.Code)
		}
		w = advanceDetailHTTP(advanceDetailFixture(), method, "7?actor_user_id=999&actor_role=ADMIN", `{"purpose":"Updated"}`, true)
		var body struct {
			Success bool
			Data    dto.AdvanceListItemResponse
			Meta    json.RawMessage
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || !body.Success || body.Data.ID != 7 || body.Data.Project.ID != 10 || body.Data.Requester.UserID != 25 || body.Data.Requester.FirstName != "Somchai" || body.Data.Amount != "15000.00" || body.Data.SubmittedAt != nil || body.Data.AdvanceCode != "ADV-PRJ001-001" || len(body.Meta) != 0 {
			t.Fatalf("%s response=%d %s", method, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), `"pm_user_id"`) || strings.Contains(w.Body.String(), `"pc_user_id"`) {
			t.Fatalf("internal membership fields leaked into response: %s", w.Body.String())
		}
	}
	for _, body := range []string{``, `{`, `null`, `[]`, `{}`, `{"amount":null}`, `{"amount":1}`, `{"purpose":12}`, `{"purpose":"  "}`, `{"amount":""}`, `{"amount":"0"}`, `{"amount":"-1"}`, `{"amount":"1.001"}`, `{"amount":"1e3"}`, `{"amount":"10000000000000"}`, `{"status":"ACTIVE"}`} {
		r := advanceDetailFixture()
		w := advanceDetailHTTP(r, http.MethodPatch, "7", body, true)
		if w.Code != http.StatusBadRequest || len(r.calls) != 0 {
			t.Fatalf("body=%s: %d %s calls=%v", body, w.Code, w.Body.String(), r.calls)
		}
	}
	for _, tt := range []struct {
		configure func(*advanceDetailRepoStub)
		status    int
	}{
		{func(r *advanceDetailRepoStub) { r.advance.CreatedBy = 99 }, http.StatusForbidden},
		{func(r *advanceDetailRepoStub) { r.advance.Status = model.AdvanceStatusActive }, http.StatusConflict},
		{func(r *advanceDetailRepoStub) { r.project.Status = model.ProjectStatusClosed }, http.StatusConflict},
		{func(r *advanceDetailRepoStub) { r.eventErr = errors.New("database secret") }, http.StatusInternalServerError},
	} {
		r := advanceDetailFixture()
		tt.configure(r)
		w := advanceDetailHTTP(r, http.MethodPatch, "7", `{"purpose":"Updated"}`, true)
		if w.Code != tt.status || strings.Contains(w.Body.String(), "database secret") {
			t.Fatalf("response=%d %s", w.Code, w.Body.String())
		}
	}
}
