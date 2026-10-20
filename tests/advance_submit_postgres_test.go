package tests

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"gorm.io/gorm"
)

func TestAdvanceSubmitPostgresConcurrentRequestsCreateOneRevision(t *testing.T) {
	db, ctx, actor, project, _ := advanceCreationDatabase(t)
	repo := repository.NewAdvanceRepository(db)
	svc := service.NewAdvanceService(repo)
	created, err := svc.Create(ctx, actor, "PC", &dto.CreateAdvanceRequest{ProjectID: project, Purpose: "Travel", Amount: "9999999999999.99"})
	if err != nil {
		t.Fatal(err)
	}
	const workers = 8
	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.Submit(ctx, actor, "PC", created.ID)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, dto.ErrAdvanceNotSubmittable):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent submit error=%v", err)
		}
	}
	if successes != 1 || conflicts != workers-1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	revisions, err := svc.Revisions(ctx, actor, "PC", created.ID)
	if err != nil || len(revisions) != 1 || revisions[0].Revision != 1 || revisions[0].Snapshot.Amount != "9999999999999.99" || revisions[0].Snapshot.Status != model.AdvanceStatusPendingPMApproval {
		t.Fatalf("revisions=%#v %v", revisions, err)
	}
	var stored model.Advance
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SubmittedAt == nil || stored.SubmittedBy == nil || *stored.SubmittedBy != actor || !stored.SubmittedAt.Equal(revisions[0].SubmittedAt) || !stored.UpdatedAt.Equal(*stored.SubmittedAt) {
		t.Fatalf("submission metadata=%#v", stored)
	}
	var events int64
	if err := db.Model(&model.AdvanceWorkflowEvent{}).Where("advance_id = ?", created.ID).Count(&events).Error; err != nil || events != 2 {
		t.Fatalf("only CREATE and SUBMIT expected: %d %v", events, err)
	}
	if _, err := repo.GetSubmissionHistory(ctx, -1); !errors.Is(err, dto.ErrAdvanceNotFound) {
		t.Fatalf("missing history=%v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := repo.Submit(ctx, tx, &model.Advance{ID: -1, Status: model.AdvanceStatusPendingPMApproval, UpdatedBy: actor}); !errors.Is(err, dto.ErrAdvanceNotFound) {
			t.Fatalf("missing submit=%v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAdvanceRevisionsPostgresResubmissionRetainsPriorSnapshot(t *testing.T) {
	db, ctx, actor, project, _ := advanceCreationDatabase(t)
	svc := service.NewAdvanceService(repository.NewAdvanceRepository(db))
	created, err := svc.Create(ctx, actor, "PC", &dto.CreateAdvanceRequest{ProjectID: project, Purpose: "Original", Amount: "15000.00"})
	if err != nil {
		t.Fatal(err)
	}
	// An existing record with no submissions must remain distinguishable from 404.
	if revisions, err := svc.Revisions(ctx, actor, "PC", created.ID); err != nil || revisions == nil || len(revisions) != 0 {
		t.Fatalf("unsubmitted history=%#v %v", revisions, err)
	}
	if _, err := svc.Submit(ctx, actor, "PC", created.ID); err != nil {
		t.Fatal(err)
	}
	var firstJSON string
	if err := db.Raw(`SELECT after_data::text FROM advance_workflow_events WHERE advance_id=? AND action='SUBMIT'`, created.ID).Scan(&firstJSON).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE advances SET status='RETURNED_FOR_REVISION' WHERE id=?`, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	purpose, amount := "Revised", "12000.01"
	if _, err := svc.Update(ctx, actor, "PC", created.ID, &dto.UpdateAdvanceRequest{Purpose: &purpose}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, actor, "PC", created.ID, &dto.UpdateAdvanceRequest{Amount: &amount}); err != nil {
		t.Fatal(err)
	}
	var pmID int64
	if err := db.Raw(`SELECT pm_user_id FROM projects WHERE id=?`, project).Scan(&pmID).Error; err != nil {
		t.Fatal(err)
	}
	if items, err := svc.Revisions(ctx, pmID, "PM", created.ID); items != nil || !errors.Is(err, dto.ErrForbidden) {
		t.Fatalf("current private draft history=%#v %v", items, err)
	}
	if _, err := svc.Submit(ctx, actor, "PC", created.ID); err != nil {
		t.Fatal(err)
	}
	revisions, err := svc.Revisions(ctx, pmID, "PM", created.ID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("submitted history=%#v %v", revisions, err)
	}
	var original model.Advance
	if err := json.Unmarshal([]byte(firstJSON), &original); err != nil {
		t.Fatal(err)
	}
	if revisions[0].Revision != 1 || revisions[1].Revision != 2 || revisions[0].Snapshot.Purpose != original.Purpose || revisions[0].Snapshot.Amount != "15000.00" || revisions[1].Snapshot.Amount != amount || revisions[1].Snapshot.Purpose != purpose || revisions[0].SubmittedBy != actor || revisions[1].SubmittedBy != actor {
		t.Fatalf("revisions=%#v", revisions)
	}
	var unchanged string
	if err := db.Raw(`SELECT after_data::text FROM advance_workflow_events WHERE advance_id=? AND action='SUBMIT' ORDER BY id LIMIT 1`, created.ID).Scan(&unchanged).Error; err != nil || unchanged != firstJSON {
		t.Fatalf("prior snapshot changed=%v %v", unchanged != firstJSON, err)
	}
	other, err := svc.Create(ctx, actor, "PC", &dto.CreateAdvanceRequest{ProjectID: project, Purpose: "Another request", Amount: "1.00"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, actor, "PC", other.ID); err != nil {
		t.Fatal(err)
	}
	if items, err := svc.Revisions(ctx, actor, "PC", other.ID); err != nil || len(items) != 1 || items[0].Revision != 1 || items[0].Snapshot.ID != other.ID {
		t.Fatalf("revision numbering must be per Advance: %#v %v", items, err)
	}
}

func TestAdvanceSubmitPostgresRollsBackMetadataWhenAuditFails(t *testing.T) {
	db, ctx, actor, project, _ := advanceCreationDatabase(t)
	svc := service.NewAdvanceService(repository.NewAdvanceRepository(db))
	created, err := svc.Create(ctx, actor, "PC", &dto.CreateAdvanceRequest{ProjectID: project, Purpose: "Original", Amount: "15000.00"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`ALTER TABLE advance_workflow_events ADD CONSTRAINT reject_test_submissions CHECK (action <> 'SUBMIT')`).Error; err != nil {
		t.Fatal(err)
	}
	if item, err := svc.Submit(ctx, actor, "PC", created.ID); item != nil || !errors.Is(err, dto.ErrInternalServer) {
		t.Fatalf("failed submission=%#v %v", item, err)
	}
	var stored model.Advance
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.AdvanceStatusDraft || stored.SubmittedAt != nil || stored.SubmittedBy != nil || !stored.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("failed audit must roll back submission: %#v", stored)
	}
	if revisions, err := svc.Revisions(ctx, actor, "PC", created.ID); err != nil || len(revisions) != 0 {
		t.Fatalf("failed submission created revision=%#v %v", revisions, err)
	}
}
