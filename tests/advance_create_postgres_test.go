package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// All SQL in this file runs only in a disposable database explicitly selected
// for testing. Each test owns an isolated schema; application config is unused.
func advanceCreationDatabase(t *testing.T) (*gorm.DB, context.Context, int64, int64, int64) {
	t.Helper()
	url := os.Getenv("ERBM_MIGRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set ERBM_MIGRATION_TEST_DATABASE_URL for isolated PostgreSQL creation/migration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid migration test database configuration")
	}
	schema := pgx.Identifier{fmt.Sprintf("erbm_advance_create_%d", time.Now().UnixNano())}.Sanitize()
	config.RuntimeParams["search_path"] = schema
	raw := stdlib.OpenDB(*config)
	raw.SetMaxOpenConns(16)
	t.Cleanup(func() { raw.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: raw}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("could not open configured PostgreSQL test database")
	}
	db = db.WithContext(ctx)
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := raw.ExecContext(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("could not remove isolated creation test schema")
		}
	})
	for _, name := range []string{"0001_create_auth_tables.up.sql", "0002_add_employee_user_audit_fields.up.sql", "0004_create_account_activation_tokens.up.sql", "0003_create_projects.up.sql", "0005_create_advance_workflow.up.sql"} {
		advanceApplyMigration(t, db, name)
	}
	var employeeID, userID, one, two int64
	if err := db.Raw(`INSERT INTO employees(first_name,last_name,position,email,date_hired) VALUES ('Somchai','Jaidee','PC','create@example.test','2026-01-01') RETURNING id`).Scan(&employeeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`INSERT INTO users(employee_id,email,role_id,status) SELECT ?,'create@example.test',id,'ACTIVE' FROM roles WHERE code='PC' RETURNING id`, employeeID).Scan(&userID).Error; err != nil {
		t.Fatal(err)
	}
	var pmEmployeeID, pmID int64
	if err := db.Raw(`INSERT INTO employees(first_name,last_name,position,email,date_hired) VALUES ('Manager','Fixture','PM','manager-create@example.test','2026-01-01') RETURNING id`).Scan(&pmEmployeeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`INSERT INTO users(employee_id,email,role_id,status) SELECT ?,'manager-create@example.test',id,'ACTIVE' FROM roles WHERE code='PM' RETURNING id`, pmEmployeeID).Scan(&pmID).Error; err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		code string
		id   *int64
	}{{"PRJ001", &one}, {"PRJ002", &two}} {
		if err := db.Raw(`INSERT INTO projects(project_code,name,budget,pm_user_id,pc_user_id) VALUES (?, 'Project fixture',10000,?,?) RETURNING id`, target.code, pmID, userID).Scan(target.id).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db, ctx, userID, one, two
}

func advanceApplyMigration(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(content)).Error }); err != nil {
		t.Fatalf("migration %s: %v", name, err)
	}
}

func TestAdvanceCreatePostgresConcurrencyAndAtomicity(t *testing.T) {
	db, ctx, actor, one, two := advanceCreationDatabase(t)
	repo := repository.NewAdvanceRepository(db)
	svc := service.NewAdvanceService(repo)
	create := func(projectID int64) (*model.AdvanceListItem, error) {
		return svc.Create(ctx, actor, "PC", &dto.CreateAdvanceRequest{ProjectID: projectID, Purpose: "Travel", Amount: "9999999999999.99"})
	}
	first, err := create(one)
	if err != nil {
		t.Fatal(err)
	}
	activeID := first.ID
	if err := db.Exec(`UPDATE advances SET status='ACTIVE' WHERE id=?`, activeID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := create(one); err != nil {
		t.Fatal(err)
	}
	item, err := create(one)
	if err != nil || item.AdvanceCode != "ADV-PRJ001-003" || item.Amount != "9999999999999.99" || item.Status != model.AdvanceStatusDraft || item.RequesterFirstName != "Somchai" {
		t.Fatalf("draft beside ACTIVE=%#v, %v", item, err)
	}
	draftID := item.ID
	item, err = create(two)
	if err != nil || item.AdvanceCode != "ADV-PRJ002-001" {
		t.Fatalf("independent project sequence=%#v, %v", item, err)
	}

	const workers = 12
	results := make(chan *model.AdvanceListItem, workers)
	failures := make(chan error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			item, err := create(one)
			if err != nil {
				failures <- err
				return
			}
			results <- item
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Errorf("concurrent create: %v", err)
	}
	if t.Failed() {
		t.FailNow()
	}
	codes := map[string]bool{}
	for item := range results {
		if codes[item.AdvanceCode] || item.Status != model.AdvanceStatusDraft {
			t.Fatalf("duplicate/wrong-state item: %#v", item)
		}
		codes[item.AdvanceCode] = true
	}
	if len(codes) != workers || !codes["ADV-PRJ001-004"] || !codes["ADV-PRJ001-015"] {
		t.Fatalf("concurrent codes=%v", codes)
	}
	var records, events, counter, active int64
	checkCounts := func(wantRecords, wantEvents, wantCounter int64) {
		t.Helper()
		if err := db.Raw(`SELECT COUNT(*) FROM advances WHERE project_id=?`, one).Scan(&records).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Raw(`SELECT COUNT(*) FROM advance_workflow_events e JOIN advances a ON a.id=e.advance_id WHERE a.project_id=?`, one).Scan(&events).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Raw(`SELECT last_number FROM project_advance_counters WHERE project_id=?`, one).Scan(&counter).Error; err != nil {
			t.Fatal(err)
		}
		if records != wantRecords || events != wantEvents || counter != wantCounter {
			t.Fatalf("records/events/counter=%d/%d/%d", records, events, counter)
		}
	}
	checkCounts(15, 15, 15)
	if err := db.Exec(`CREATE FUNCTION fail_advance_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit fixture failure'; END; $$;
		CREATE TRIGGER fail_advance_event BEFORE INSERT ON advance_workflow_events FOR EACH ROW EXECUTE FUNCTION fail_advance_event();`).Error; err != nil {
		t.Fatal(err)
	}
	if item, err := create(one); !errors.Is(err, dto.ErrInternalServer) || item != nil {
		t.Fatalf("audit failure=%#v, %v", item, err)
	}
	checkCounts(15, 15, 15)
	if err := db.Exec(`DROP TRIGGER fail_advance_event ON advance_workflow_events; DROP FUNCTION fail_advance_event();`).Error; err != nil {
		t.Fatal(err)
	}
	item, err = create(one)
	if err != nil || item.AdvanceCode != "ADV-PRJ001-016" {
		t.Fatalf("rolled-back allocation=%#v, %v", item, err)
	}
	checkCounts(16, 16, 16)

	// The original one-ACTIVE partial unique index still guards activation.
	err = db.Exec(`UPDATE advances SET status='ACTIVE' WHERE id=?`, draftID).Error
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("second ACTIVE should be rejected: %v", err)
	}
	if err := db.Exec(`UPDATE advances SET status='CLOSED' WHERE id=?`, activeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE advances SET status='ACTIVE' WHERE id=?`, draftID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT COUNT(*) FROM advances WHERE project_id=? AND status='ACTIVE'`, one).Scan(&active).Error; err != nil || active != 1 {
		t.Fatalf("active count=%d, %v", active, err)
	}

	// Stored codes remain unchanged even if a Project is renamed directly.
	if err := db.Exec(`UPDATE projects SET project_code='RENAMED' WHERE id=?`, one).Error; err != nil {
		t.Fatal(err)
	}
	items, total, err := repo.List(ctx, model.AdvanceFilter{ActorUserID: actor, ActorRole: "PC", ProjectID: &one, Page: 1, PageSize: 100})
	if err != nil || total != 16 {
		t.Fatalf("list=%d, %v", total, err)
	}
	for _, item := range items {
		if item.ID == draftID && item.AdvanceCode != "ADV-PRJ001-003" {
			t.Fatalf("code changed=%#v", item)
		}
	}
	if err := db.Exec(`UPDATE projects SET status='CLOSED' WHERE id=?`, two).Error; err != nil {
		t.Fatal(err)
	}
	if item, err := create(two); !errors.Is(err, dto.ErrProjectClosed) || item != nil {
		t.Fatalf("closed project=%#v, %v", item, err)
	}
}

func TestAdvanceWorkflowMigrationRecreatesIdentifiers(t *testing.T) {
	db, ctx, actor, one, two := advanceCreationDatabase(t)
	svc := service.NewAdvanceService(repository.NewAdvanceRepository(db))
	create := func(project int64) (*model.AdvanceListItem, error) {
		return svc.Create(ctx, actor, "PC", &dto.CreateAdvanceRequest{ProjectID: project, Purpose: "Migration fixture", Amount: "0.01"})
	}
	for cycle := 0; cycle < 2; cycle++ {
		first, err := create(one)
		if err != nil || first.AdvanceCode != "ADV-PRJ001-001" {
			t.Fatalf("fresh project code=%#v, %v", first, err)
		}
		other, err := create(two)
		if err != nil || other.AdvanceCode != "ADV-PRJ002-001" {
			t.Fatalf("independent project code=%#v, %v", other, err)
		}
		if err := db.Exec(`UPDATE project_advance_counters SET last_number=999 WHERE project_id=?`, one).Error; err != nil {
			t.Fatal(err)
		}
		last, err := create(one)
		if err != nil || last.AdvanceCode != "ADV-PRJ001-1000" {
			t.Fatalf("expanded number=%#v, %v", last, err)
		}
		err = db.Exec(`INSERT INTO advances(advance_code,project_id,purpose,amount,created_by,updated_by) VALUES ('ADV-PRJ001-001',?,'Duplicate code',1,?,?)`, one, actor, actor).Error
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_advances_advance_code" {
			t.Fatalf("duplicate code error=%v", err)
		}
		advanceApplyMigration(t, db, "0005_create_advance_workflow.down.sql")
		var tables, projects int64
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('advances','advance_workflow_events','project_advance_counters')`).Scan(&tables).Error; err != nil || tables != 0 {
			t.Fatalf("workflow tables after down=%d, %v", tables, err)
		}
		if err := db.Raw(`SELECT COUNT(*) FROM projects`).Scan(&projects).Error; err != nil || projects != 2 {
			t.Fatalf("baseline projects after down=%d, %v", projects, err)
		}
		advanceApplyMigration(t, db, "0005_create_advance_workflow.up.sql")
	}
}
