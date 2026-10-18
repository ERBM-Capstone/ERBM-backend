package tests

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"gorm.io/gorm"
)

func TestAdvanceDetailPostgresConcurrentPatchesPreserveFieldsAndHistory(t *testing.T) {
	db, ctx, actor, project, _ := advanceCreationDatabase(t)
	repo := repository.NewAdvanceRepository(db)
	svc := service.NewAdvanceService(repo)
	created, err := svc.Create(ctx, actor, "PC", &dto.CreateAdvanceRequest{ProjectID: project, Purpose: "Original", Amount: "10000.00"})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)
	if err := db.Exec(`UPDATE advances SET status='RETURNED_FOR_REVISION', submitted_at=?, submitted_by=? WHERE id=?`, when, actor, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	purpose, amount := "Travel and hotel", "12000.01"
	start := make(chan struct{})
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for _, req := range []dto.UpdateAdvanceRequest{{Purpose: &purpose}, {Amount: &amount}} {
		wg.Add(1)
		go func(req dto.UpdateAdvanceRequest) {
			defer wg.Done()
			<-start
			_, err := svc.Update(ctx, actor, "PC", created.ID, &req)
			failures <- err
		}(req)
	}
	close(start)
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	item, err := svc.GetByID(ctx, actor, "PC", created.ID)
	if err != nil || item.Purpose != purpose || item.Amount != amount || item.Status != model.AdvanceStatusDraft || item.AdvanceCode != created.AdvanceCode || item.SubmittedAt == nil || !item.SubmittedAt.Equal(when) {
		t.Fatalf("current=%#v err=%v", item, err)
	}
	var events []model.AdvanceWorkflowEvent
	if err := db.Where("advance_id = ?", created.ID).Order("id").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events=%#v", events)
	}
	var initial, first, beforeSecond, last model.Advance
	if err := json.Unmarshal([]byte(events[0].AfterData), &initial); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(events[1].AfterData), &first); err != nil {
		t.Fatal(err)
	}
	if events[2].BeforeData == nil {
		t.Fatal("missing second patch before snapshot")
	}
	if err := json.Unmarshal([]byte(*events[2].BeforeData), &beforeSecond); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(events[2].AfterData), &last); err != nil {
		t.Fatal(err)
	}
	if initial.Amount != "10000.00" || initial.Purpose != "Original" || initial.Status != model.AdvanceStatusDraft || first.Purpose != beforeSecond.Purpose || first.Amount != beforeSecond.Amount || first.Status != beforeSecond.Status || last.Purpose != purpose || last.Amount != amount || events[1].PreviousStatus == nil || *events[1].PreviousStatus != model.AdvanceStatusReturnedForRevision || events[2].PreviousStatus == nil || *events[2].PreviousStatus != model.AdvanceStatusDraft {
		t.Fatalf("snapshot chain=%#v", events)
	}
	for _, role := range []string{"ADMIN", "ACCOUNTING", "PM"} {
		if item, err := svc.GetByID(ctx, actor+1000, role, created.ID); item != nil || !errors.Is(err, dto.ErrForbidden) {
			t.Fatalf("%s saw private draft: %#v %v", role, item, err)
		}
	}
	if _, err := repo.GetByID(ctx, -1); !errors.Is(err, dto.ErrAdvanceNotFound) {
		t.Fatalf("missing read=%v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := repo.GetByIDForUpdate(ctx, tx, -1); !errors.Is(err, dto.ErrAdvanceNotFound) {
			t.Fatalf("missing lock=%v", err)
		}
		if err := repo.Update(ctx, tx, &model.Advance{ID: -1, Amount: "1.00", Purpose: "Missing", Status: model.AdvanceStatusDraft, UpdatedBy: actor}); !errors.Is(err, dto.ErrAdvanceNotFound) {
			t.Fatalf("missing update=%v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAdvanceDetailPostgresRollsBackPatchWhenAuditFails(t *testing.T) {
	db, ctx, actor, project, _ := advanceCreationDatabase(t)
	svc := service.NewAdvanceService(repository.NewAdvanceRepository(db))
	created, err := svc.Create(ctx, actor, "PC", &dto.CreateAdvanceRequest{ProjectID: project, Purpose: "Original", Amount: "10000.00"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE advances SET status='RETURNED_FOR_REVISION' WHERE id=?`, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	// A database rejection after UPDATE proves that mutation and event share
	// one real transaction, including the revision-to-draft status change.
	if err := db.Exec(`ALTER TABLE advance_workflow_events ADD CONSTRAINT reject_test_updates CHECK (action <> 'UPDATE')`).Error; err != nil {
		t.Fatal(err)
	}
	purpose := "Must roll back"
	item, err := svc.Update(ctx, actor, "PC", created.ID, &dto.UpdateAdvanceRequest{Purpose: &purpose})
	if item != nil || !errors.Is(err, dto.ErrInternalServer) {
		t.Fatalf("patch=%#v %v", item, err)
	}
	current, err := svc.GetByID(ctx, actor, "PC", created.ID)
	if err != nil || current.Purpose != "Original" || current.Amount != "10000.00" || current.Status != model.AdvanceStatusReturnedForRevision || !current.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("rollback=%#v %v", current, err)
	}
	var count int64
	if err := db.Model(&model.AdvanceWorkflowEvent{}).Where("advance_id = ?", created.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
}
