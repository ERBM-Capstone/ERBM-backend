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

func TestAdvanceDetailRepositoryUsesContextLocksAndWhitelistedWrites(t *testing.T) {
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
			t.Error("database operation lost request context")
		}
		queries = append(queries, advanceCapturedQuery{sql: db.Statement.SQL.String(), args: append([]any(nil), db.Statement.Vars...)})
	}
	if err := db.Callback().Query().After("gorm:query").Register("capture_detail_query", capture); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().After("gorm:update").Register("capture_detail_update", capture); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewAdvanceRepository(db)
	if _, err := repo.GetByID(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetProjectForUpdate(ctx, db, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByIDForUpdate(ctx, db, 7); err != nil {
		t.Fatal(err)
	}
	// DryRun reports no affected rows. Actual missing-row behavior is also
	// covered against PostgreSQL in the opt-in integration test.
	err = repo.Update(ctx, db, &model.Advance{
		ID: 7, AdvanceCode: "FORGED", ProjectID: 99, CreatedBy: 99,
		Purpose: "a' OR 1=1 --", Amount: "9999999999999.99", Status: model.AdvanceStatusDraft,
		UpdatedBy: 25, UpdatedAt: time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, dto.ErrAdvanceNotFound) {
		t.Fatal(err)
	}
	if len(queries) != 4 {
		t.Fatalf("queries=%#v", queries)
	}
	if !strings.Contains(queries[0].sql, "a.amount::text") || !strings.Contains(queries[0].sql, "a.id = $") || !strings.Contains(queries[0].sql, "JOIN projects") {
		t.Fatalf("detail query=%s", queries[0].sql)
	}
	if !strings.Contains(queries[0].sql, "p.pm_user_id") || !strings.Contains(queries[0].sql, "p.pc_user_id") {
		t.Fatalf("detail read must include membership in the same snapshot: %s", queries[0].sql)
	}
	for _, i := range []int{1, 2} {
		if !strings.Contains(queries[i].sql, "FOR UPDATE") || !strings.Contains(queries[i].sql, "id = $") {
			t.Fatalf("lock=%s", queries[i].sql)
		}
	}
	update := queries[3]
	for _, field := range []string{"purpose", "amount", "status", "updated_by", "updated_at"} {
		if !strings.Contains(update.sql, `"`+field+`"=`) {
			t.Fatalf("missing field %s: %s", field, update.sql)
		}
	}
	for _, forbidden := range []string{"advance_code", "project_id", "created_by", "created_at", "submitted_at", "submitted_by", "FORGED", "a' OR 1=1 --"} {
		if strings.Contains(update.sql, forbidden) {
			t.Fatalf("unexpected field/interpolation %s: %s", forbidden, update.sql)
		}
	}
	foundPurpose, foundAmount := false, false
	for _, arg := range update.args {
		if arg == "a' OR 1=1 --" {
			foundPurpose = true
		}
		if arg == "9999999999999.99" {
			foundAmount = true
		}
	}
	if !foundPurpose || !foundAmount || !strings.Contains(update.sql, "WHERE id = $") {
		t.Fatalf("unbound patch=%#v", update)
	}
}
