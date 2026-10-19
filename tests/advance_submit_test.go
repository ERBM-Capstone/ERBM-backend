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

type advanceSubmissionRepoStub struct {
	*advanceDetailRepoStub
	submitErr, historyErr error
	historyCalls          int
}

func advanceSubmissionFixture() *advanceSubmissionRepoStub {
	return &advanceSubmissionRepoStub{advanceDetailRepoStub: advanceDetailFixture()}
}

func (r *advanceSubmissionRepoStub) Submit(_ context.Context, _ *gorm.DB, a *model.Advance) error {
	r.calls = append(r.calls, "submit")
	if r.submitErr != nil {
		return r.submitErr
	}
	r.advance = *a
	return nil
}

func (r *advanceSubmissionRepoStub) WithTx(ctx context.Context, fn func(*gorm.DB) error) error {
	before := r.advance
	events := append([]model.AdvanceWorkflowEvent(nil), r.events...)
	err := r.advanceDetailRepoStub.WithTx(ctx, fn)
	if err != nil {
		r.advance, r.events = before, events
	}
	return err
}

func (r *advanceSubmissionRepoStub) GetSubmissionHistory(_ context.Context, id int64) (*model.AdvanceSubmissionHistory, error) {
	r.historyCalls++
	if r.historyErr != nil {
		return nil, r.historyErr
	}
	if id != r.advance.ID {
		return nil, dto.ErrAdvanceNotFound
	}
	h := &model.AdvanceSubmissionHistory{Advance: *r.item()}
	for _, e := range r.events {
		if e.Action == model.AdvanceActionSubmit {
			h.Submissions = append(h.Submissions, model.AdvanceSubmission{ActorUserID: e.ActorUserID, AfterData: e.AfterData, OccurredAt: e.OccurredAt})
		}
	}
	return h, nil
}

func TestAdvanceSubmitPreservesRequestAndRecordsAtomicSnapshot(t *testing.T) {
	r := advanceSubmissionFixture()
	svc := service.NewAdvanceService(r)
	item, err := svc.Submit(context.Background(), 25, " pc ", 7)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != model.AdvanceStatusPendingPMApproval || item.Amount != "15000.00" || item.Purpose != "Travel" || item.AdvanceCode != "ADV-PRJ001-001" || item.SubmittedAt == nil || r.advance.SubmittedBy == nil || *r.advance.SubmittedBy != 25 || r.advance.UpdatedBy != 25 || !item.UpdatedAt.Equal(*item.SubmittedAt) {
		t.Fatalf("submission=%#v persisted=%#v", item, r.advance)
	}
	if got := strings.Join(r.calls, ","); got != "lookup,transaction,lock project,lock advance,submit,audit,response" {
		t.Fatalf("lock/transaction order=%s", got)
	}
	if len(r.events) != 1 {
		t.Fatalf("events=%#v", r.events)
	}
	e := r.events[0]
	if e.BeforeData == nil || e.PreviousStatus == nil || *e.PreviousStatus != model.AdvanceStatusDraft || e.ResultingStatus != model.AdvanceStatusPendingPMApproval || e.Action != model.AdvanceActionSubmit || e.ActorUserID != 25 || e.AdvanceID != 7 || !e.OccurredAt.Equal(*item.SubmittedAt) {
		t.Fatalf("event=%#v", e)
	}
	var before, after model.Advance
	if err := json.Unmarshal([]byte(*e.BeforeData), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(e.AfterData), &after); err != nil {
		t.Fatal(err)
	}
	if before.Status != model.AdvanceStatusDraft || before.SubmittedAt != nil || after.Status != model.AdvanceStatusPendingPMApproval || after.SubmittedBy == nil || *after.SubmittedBy != 25 || after.Amount != before.Amount || after.Purpose != before.Purpose || after.SubmittedAt == nil || !after.SubmittedAt.Equal(e.OccurredAt) {
		t.Fatalf("before=%#v after=%#v", before, after)
	}
	if duplicate, err := svc.Submit(context.Background(), 25, "PC", 7); duplicate != nil || !errors.Is(err, dto.ErrAdvanceNotSubmittable) || len(r.events) != 1 {
		t.Fatalf("duplicate=%#v err=%v events=%d", duplicate, err, len(r.events))
	}
}

func TestAdvanceSubmitRejectsActorStateAndRollsBackErrors(t *testing.T) {
	for _, tt := range []struct {
		name, role string
		actor, id  int64
		configure  func(*advanceSubmissionRepoStub)
		want       error
	}{
		{"anonymous", "PC", 0, 7, nil, dto.ErrUnauthorized},
		{"admin", "ADMIN", 25, 7, nil, dto.ErrForbidden},
		{"manager", "PM", 25, 7, nil, dto.ErrForbidden},
		{"accounting", "ACCOUNTING", 25, 7, nil, dto.ErrForbidden},
		{"invalid ID", "PC", 25, 0, nil, dto.ErrBadRequest},
		{"missing", "PC", 25, 99, nil, dto.ErrAdvanceNotFound},
		{"other creator", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.advance.CreatedBy = 99 }, dto.ErrForbidden},
		{"former PC", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { pc := int64(99); r.project.PCUserID = &pc }, dto.ErrForbidden},
		{"unassigned Project", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.project.PCUserID = nil }, dto.ErrForbidden},
		{"closed Project", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.project.Status = model.ProjectStatusClosed }, dto.ErrProjectClosed},
		{"returned needs edit", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.advance.Status = model.AdvanceStatusReturnedForRevision }, dto.ErrAdvanceNotSubmittable},
		{"pending", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.advance.Status = model.AdvanceStatusPendingPMApproval }, dto.ErrAdvanceNotSubmittable},
		{"approved", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.advance.Status = model.AdvanceStatusApproved }, dto.ErrAdvanceNotSubmittable},
		{"active", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.advance.Status = model.AdvanceStatusActive }, dto.ErrAdvanceNotSubmittable},
		{"closed", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.advance.Status = model.AdvanceStatusClosed }, dto.ErrAdvanceNotSubmittable},
		{"empty purpose", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.advance.Purpose = " " }, dto.ErrBadRequest},
		{"invalid money", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.advance.Amount = "0.00" }, dto.ErrBadRequest},
		{"lookup failure", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.lookupErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"project lock failure", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.projectErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"advance lock missing", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.lockErr = dto.ErrAdvanceNotFound }, dto.ErrAdvanceNotFound},
		{"submit failure", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.submitErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"audit failure", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.eventErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"response failure", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.readErr = errors.New("database secret") }, dto.ErrInternalServer},
		{"transaction failure", "PC", 25, 7, func(r *advanceSubmissionRepoStub) { r.txErr = errors.New("database secret") }, dto.ErrInternalServer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := advanceSubmissionFixture()
			if tt.configure != nil {
				tt.configure(r)
			}
			before := r.advance
			item, err := service.NewAdvanceService(r).Submit(context.Background(), tt.actor, tt.role, tt.id)
			if item != nil || !errors.Is(err, tt.want) || r.advance != before || len(r.events) != 0 {
				t.Fatalf("item=%#v err=%v want=%v persisted=%#v events=%#v", item, err, tt.want, r.advance, r.events)
			}
		})
	}
}

func TestAdvanceRevisionsNumberOnlySubmissionsAndPreserveHistoricalMoney(t *testing.T) {
	r := advanceSubmissionFixture()
	svc := service.NewAdvanceService(r)
	ctx := context.Background()
	if revisions, err := svc.Revisions(ctx, 25, "PC", 7); err != nil || revisions == nil || len(revisions) != 0 {
		t.Fatalf("unsubmitted=%#v %v", revisions, err)
	}
	if _, err := svc.Submit(ctx, 25, "PC", 7); err != nil {
		t.Fatal(err)
	}
	firstJSON := r.events[0].AfterData
	firstSubmission := *r.advance.SubmittedAt
	// PM return is outside this task; seed its resulting state before editing.
	r.advance.Status = model.AdvanceStatusReturnedForRevision
	purpose, amount := "Revised trip", "9999999999999.99"
	if _, err := svc.Update(ctx, 25, "PC", 7, &dto.UpdateAdvanceRequest{Purpose: &purpose, Amount: &amount}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, 25, "PC", 7); err != nil {
		t.Fatal(err)
	}
	revisions, err := svc.Revisions(ctx, 30, "PM", 7)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions=%#v %v", revisions, err)
	}
	if revisions[0].Revision != 1 || revisions[1].Revision != 2 || revisions[0].Snapshot.Amount != "15000.00" || revisions[0].Snapshot.Purpose != "Travel" || revisions[1].Snapshot.Amount != amount || revisions[1].Snapshot.Purpose != purpose || !revisions[0].SubmittedAt.Equal(firstSubmission) || revisions[0].SubmittedBy != 25 || revisions[1].Snapshot.Status != model.AdvanceStatusPendingPMApproval || r.events[0].AfterData != firstJSON {
		t.Fatalf("historical snapshots=%#v", revisions)
	}
	if len(r.events) != 3 || r.events[1].Action != model.AdvanceActionUpdate {
		t.Fatalf("audit edits must not count as revisions: %#v", r.events)
	}
}

func TestAdvanceRevisionsEnforcesCurrentVisibilityAndSafeErrors(t *testing.T) {
	for _, tt := range []struct {
		name, role string
		actor      int64
		status     model.AdvanceStatus
		want       error
	}{
		{"creator draft", "PC", 25, model.AdvanceStatusDraft, nil},
		{"other PC draft", "PC", 26, model.AdvanceStatusDraft, dto.ErrForbidden},
		{"manager draft", "PM", 30, model.AdvanceStatusDraft, dto.ErrForbidden},
		{"admin draft", "ADMIN", 40, model.AdvanceStatusDraft, dto.ErrForbidden},
		{"accounting draft", "ACCOUNTING", 40, model.AdvanceStatusDraft, dto.ErrForbidden},
		{"assigned manager", "PM", 30, model.AdvanceStatusPendingPMApproval, nil},
		{"other manager", "PM", 31, model.AdvanceStatusPendingPMApproval, dto.ErrForbidden},
		{"assigned PC", "PC", 25, model.AdvanceStatusPendingPMApproval, nil},
		{"former PC", "PC", 26, model.AdvanceStatusPendingPMApproval, dto.ErrForbidden},
		{"admin", "ADMIN", 40, model.AdvanceStatusClosed, nil},
		{"accounting", "ACCOUNTING", 40, model.AdvanceStatusActive, nil},
		{"anonymous", "PC", 0, model.AdvanceStatusDraft, dto.ErrUnauthorized},
		{"unknown role", "EMPLOYEE", 25, model.AdvanceStatusDraft, dto.ErrForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := advanceSubmissionFixture()
			r.advance.Status = tt.status
			items, err := service.NewAdvanceService(r).Revisions(context.Background(), tt.actor, tt.role, 7)
			if !errors.Is(err, tt.want) || (tt.want != nil && items != nil) {
				t.Fatalf("items=%#v err=%v want=%v", items, err, tt.want)
			}
			if len(r.calls) != 0 {
				t.Fatalf("history must use its own single visibility snapshot: %v", r.calls)
			}
		})
	}
	for _, tt := range []struct {
		id    int64
		cause error
		want  error
	}{
		{0, nil, dto.ErrBadRequest}, {-1, nil, dto.ErrBadRequest}, {99, nil, dto.ErrAdvanceNotFound},
		{7, errors.New("database secret"), dto.ErrInternalServer},
	} {
		r := advanceSubmissionFixture()
		r.historyErr = tt.cause
		if items, err := service.NewAdvanceService(r).Revisions(context.Background(), 25, "PC", tt.id); items != nil || !errors.Is(err, tt.want) {
			t.Fatalf("items=%#v err=%v want=%v", items, err, tt.want)
		}
	}
	r := advanceSubmissionFixture()
	r.events = []model.AdvanceWorkflowEvent{{Action: model.AdvanceActionSubmit, AfterData: "invalid JSON"}}
	if items, err := service.NewAdvanceService(r).Revisions(context.Background(), 25, "PC", 7); items != nil || !errors.Is(err, dto.ErrInternalServer) {
		t.Fatalf("corrupt history=%#v %v", items, err)
	}
}

func submissionHTTP(r *advanceSubmissionRepoStub, method, path, body string, actor int64, role string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if actor > 0 {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextKeyUserID, actor)
			c.Set(middleware.ContextKeyRole, role)
		})
	}
	h := handler.NewAdvanceHandler(service.NewAdvanceService(r))
	router.POST("/api/v1/advances/:id/submit", h.Submit)
	router.GET("/api/v1/advances/:id/revisions", h.Revisions)
	req := httptest.NewRequest(method, "/api/v1/advances/"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAdvanceSubmissionHTTPValidationEnvelopeAndTrustedActor(t *testing.T) {
	for _, endpoint := range []struct{ method, suffix string }{{http.MethodPost, "/submit"}, {http.MethodGet, "/revisions"}} {
		for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
			r := advanceSubmissionFixture()
			w := submissionHTTP(r, endpoint.method, id+endpoint.suffix, "", 25, "PC")
			if w.Code != http.StatusBadRequest || len(r.calls) != 0 || r.historyCalls != 0 {
				t.Fatalf("%s: %d %s", id+endpoint.suffix, w.Code, w.Body.String())
			}
		}
		for _, tt := range []struct {
			id    string
			actor int64
			want  int
		}{{"7", 0, http.StatusUnauthorized}, {"99", 25, http.StatusNotFound}} {
			w := submissionHTTP(advanceSubmissionFixture(), endpoint.method, tt.id+endpoint.suffix, "", tt.actor, "PC")
			if w.Code != tt.want {
				t.Fatalf("%s: %d %s", endpoint.suffix, w.Code, w.Body.String())
			}
		}
	}
	r := advanceSubmissionFixture()
	w := submissionHTTP(r, http.MethodGet, "7/revisions", "", 25, "PC")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("empty revisions=%d %s", w.Code, w.Body.String())
	}
	w = submissionHTTP(r, http.MethodPost, "7/submit?actor_user_id=999", `{"submitted_by":999,"status":"ACTIVE","amount":"1.00"}`, 25, "PC")
	var submitted struct {
		Success bool                        `json:"success"`
		Data    dto.AdvanceListItemResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || !submitted.Success || submitted.Data.Status != model.AdvanceStatusPendingPMApproval || submitted.Data.SubmittedAt == nil || submitted.Data.Amount != "15000.00" || r.advance.SubmittedBy == nil || *r.advance.SubmittedBy != 25 {
		t.Fatalf("submit=%d %s", w.Code, w.Body.String())
	}
	w = submissionHTTP(r, http.MethodGet, "7/revisions", "", 30, "PM")
	var history struct {
		Success bool                          `json:"success"`
		Data    []dto.AdvanceRevisionResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || !history.Success || len(history.Data) != 1 || history.Data[0].Revision != 1 || history.Data[0].Snapshot.Amount != "15000.00" || history.Data[0].SubmittedBy != 25 || history.Data[0].SubmittedAt != *submitted.Data.SubmittedAt || strings.Contains(w.Body.String(), "pm_user_id") || strings.Contains(w.Body.String(), "before_data") || strings.Contains(w.Body.String(), `"meta"`) {
		t.Fatalf("revisions=%d %s", w.Code, w.Body.String())
	}
	for _, tt := range []struct {
		configure func(*advanceSubmissionRepoStub)
		want      int
	}{
		{func(r *advanceSubmissionRepoStub) { r.advance.Status = model.AdvanceStatusActive }, http.StatusConflict},
		{func(r *advanceSubmissionRepoStub) { r.project.Status = model.ProjectStatusClosed }, http.StatusConflict},
		{func(r *advanceSubmissionRepoStub) { r.advance.CreatedBy = 99 }, http.StatusForbidden},
		{func(r *advanceSubmissionRepoStub) { r.eventErr = errors.New("database secret") }, http.StatusInternalServerError},
	} {
		r := advanceSubmissionFixture()
		tt.configure(r)
		w := submissionHTTP(r, http.MethodPost, "7/submit", "", 25, "PC")
		if w.Code != tt.want || strings.Contains(w.Body.String(), "database secret") {
			t.Fatalf("error=%d %s", w.Code, w.Body.String())
		}
	}
}

func TestAdvanceSubmissionRoutesEnforceAuthenticationAndRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := jwtmanager.NewManager("advance-submit-access-key-with-at-least-32-bytes", "advance-submit-refresh-key-with-at-least-32-bytes", "advance-submit", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r := appRouter.New(nil, tokens, false, service.LoginPolicy{}, service.ActivationConfig{}, nil)
	for _, endpoint := range []struct{ method, suffix string }{{http.MethodPost, "/submit"}, {http.MethodGet, "/revisions"}} {
		for _, role := range []string{"", "EMPLOYEE", "ADMIN", "ACCOUNTING", "PM", "PC"} {
			req := httptest.NewRequest(endpoint.method, "/api/v1/advances/invalid"+endpoint.suffix, nil)
			want := http.StatusBadRequest
			if role == "" {
				want = http.StatusUnauthorized
			} else {
				token, err := tokens.GenerateAccessToken(25, role)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
				if role == "EMPLOYEE" || (endpoint.method == http.MethodPost && role != "PC") {
					want = http.StatusForbidden
				}
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != want {
				t.Fatalf("%s %s: %d want=%d %s", endpoint.suffix, role, w.Code, want, w.Body.String())
			}
		}
	}
}
