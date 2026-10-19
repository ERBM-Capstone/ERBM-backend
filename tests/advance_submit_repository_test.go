package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAdvanceSubmissionRepositoryBindsContextAndKeepsHistoryVisibilityAtomic(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var queries []advanceCapturedQuery
	capture := func(db *gorm.DB) {
		if db.Statement.Context != ctx {
			t.Error("lost request context")
		}
		queries = append(queries, advanceCapturedQuery{sql: db.Statement.SQL.String(), args: append([]any(nil), db.Statement.Vars...)})
	}
	if err := db.Callback().Query().After("gorm:query").Register("capture_submission_history", capture); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().After("gorm:update").Register("capture_submission_write", capture); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewAdvanceRepository(db)
	when, actor := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC), int64(25)
	err = repo.Submit(ctx, db, &model.Advance{
		ID: 7, Status: model.AdvanceStatusPendingPMApproval, SubmittedAt: &when, SubmittedBy: &actor,
		UpdatedAt: when, UpdatedBy: actor, Purpose: "forged", Amount: "1.00", AdvanceCode: "FORGED", ProjectID: 99, CreatedBy: 99,
	})
	if !errors.Is(err, dto.ErrAdvanceNotFound) {
		t.Fatalf("DryRun missing affected rows=%v", err)
	}
	if _, err := repo.GetSubmissionHistory(ctx, 7); !errors.Is(err, dto.ErrAdvanceNotFound) {
		t.Fatalf("DryRun no rows=%v", err)
	}
	if len(queries) != 2 {
		t.Fatalf("history must use one statement: %#v", queries)
	}
	for _, field := range []string{"status", "submitted_at", "submitted_by", "updated_at", "updated_by"} {
		if !strings.Contains(queries[0].sql, `"`+field+`"=`) {
			t.Fatalf("missing metadata %s: %s", field, queries[0].sql)
		}
	}
	for _, field := range []string{"purpose", "amount", "advance_code", "project_id", "created_by", "created_at", "FORGED"} {
		if strings.Contains(queries[0].sql, field) {
			t.Fatalf("submit changed request contents %s: %s", field, queries[0].sql)
		}
	}
	query := queries[1]
	for _, fragment := range []string{"LEFT JOIN advance_workflow_events", "e.advance_id = a.id", "e.action = $", "p.pm_user_id", "p.pc_user_id", "a.status", "a.created_by", "e.after_data::text", "a.id = $", "ORDER BY e.id ASC"} {
		if !strings.Contains(query.sql, fragment) {
			t.Fatalf("missing %q in history query: %s", fragment, query.sql)
		}
	}
	if len(query.args) != 2 || query.args[0] != model.AdvanceActionSubmit || query.args[1] != int64(7) {
		t.Fatalf("history scope must be bound: %#v", query)
	}
}
