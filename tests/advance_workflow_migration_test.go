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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Use an explicitly configured disposable PostgreSQL database. Tests never
// load .env or fall back to the application's DATABASE_URL.
func TestAdvanceWorkflowMigration(t *testing.T) {
	readMigration := func(name string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join("..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		return string(content)
	}
	upSQL := readMigration("0005_create_advance_workflow.up.sql")
	downSQL := readMigration("0005_create_advance_workflow.down.sql")

	databaseURL := os.Getenv("ERBM_MIGRATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ERBM_MIGRATION_TEST_DATABASE_URL to a disposable PostgreSQL database for migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	schema := fmt.Sprintf("erbm_advance_migration_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid migration test database configuration")
	}
	poolConfig.MaxConns = 2
	poolConfig.ConnConfig.RuntimeParams["search_path"] = quotedSchema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("could not initialize the migration test connection pool")
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal("could not create the isolated migration test schema")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Error("could not remove the isolated migration test schema")
		}
	})

	applySQL := func(sql string) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, sql); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	for _, name := range []string{
		"0001_create_auth_tables.up.sql",
		"0002_add_employee_user_audit_fields.up.sql",
		"0003_create_projects.up.sql",
		"0004_create_account_activation_tokens.up.sql",
	} {
		if err := applySQL(readMigration(name)); err != nil {
			t.Fatalf("apply baseline migration %s: %v", name, err)
		}
	}
	if err := applySQL(upSQL); err != nil {
		t.Fatalf("apply advance workflow migration: %v", err)
	}

	var employeeID, actorID int64
	err = pool.QueryRow(ctx, `INSERT INTO employees
		(first_name, last_name, position, email, date_hired)
		VALUES ('Migration', 'Fixture', 'Project Manager', 'migration@example.test', '2026-01-01')
		RETURNING id`).Scan(&employeeID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO users (employee_id, email, role_id, status)
		SELECT $1, 'migration@example.test', id, 'ACTIVE' FROM roles WHERE code = 'PM'
		RETURNING id`, employeeID).Scan(&actorID)
	if err != nil {
		t.Fatal(err)
	}
	projectCounter := 0
	newProject := func() int64 {
		t.Helper()
		projectCounter++
		var id int64
		err := pool.QueryRow(ctx, `INSERT INTO projects (project_code, name, budget, pm_user_id)
			VALUES ($1, 'Migration fixture', 10000.00, $2) RETURNING id`,
			fmt.Sprintf("MIG-%d", projectCounter), actorID).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	const insertAdvance = `INSERT INTO advances
		(project_id, purpose, amount, currency_code, status, created_by, updated_by)
		VALUES ($1, 'Migration fixture', $2, 'THB', $3, $4, $4) RETURNING id`
	expectSQLState := func(t *testing.T, err error, state string) {
		t.Helper()
		var pgError *pgconn.PgError
		if !errors.As(err, &pgError) || pgError.Code != state {
			t.Fatalf("expected PostgreSQL state %s, got %v", state, err)
		}
	}

	t.Run("draft defaults and exact monetary amount", func(t *testing.T) {
		var status, amount, currency string
		err := pool.QueryRow(ctx, `INSERT INTO advances
			(project_id, purpose, amount, currency_code, created_by, updated_by)
			VALUES ($1, 'Exact monetary fixture', '9999999999999.99', 'THB', $2, $2)
			RETURNING status, amount::text, currency_code`, newProject(), actorID).
			Scan(&status, &amount, &currency)
		if err != nil {
			t.Fatal(err)
		}
		if status != "DRAFT" || amount != "9999999999999.99" || currency != "THB" {
			t.Fatalf("stored values = %q, %q, %q", status, amount, currency)
		}
	})

	t.Run("only active advances occupy the project slot", func(t *testing.T) {
		projectID := newProject()
		for _, status := range []string{"DRAFT", "DRAFT", "PENDING_PM_APPROVAL", "APPROVED", "RETURNED_FOR_REVISION", "CLOSED", "ACTIVE"} {
			var id int64
			if err := pool.QueryRow(ctx, insertAdvance, projectID, "100.00", status, actorID).Scan(&id); err != nil {
				t.Fatalf("insert %s: %v", status, err)
			}
		}
		_, err := pool.Exec(ctx, insertAdvance, projectID, "100.00", "ACTIVE", actorID)
		expectSQLState(t, err, "23505")
		if _, err := pool.Exec(ctx, insertAdvance, newProject(), "100.00", "ACTIVE", actorID); err != nil {
			t.Fatalf("active advance in another project: %v", err)
		}
	})

	t.Run("concurrent activation leaves one active advance", func(t *testing.T) {
		projectID := newProject()
		var ids [2]int64
		for i := range ids {
			if err := pool.QueryRow(ctx, insertAdvance, projectID, "100.00", "APPROVED", actorID).Scan(&ids[i]); err != nil {
				t.Fatal(err)
			}
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		for _, id := range ids {
			go func(id int64) {
				ready.Done()
				<-start
				_, err := pool.Exec(ctx, "UPDATE advances SET status = 'ACTIVE' WHERE id = $1", id)
				results <- err
			}(id)
		}
		ready.Wait()
		close(start)
		successes := 0
		for range ids {
			select {
			case err := <-results:
				if err == nil {
					successes++
				} else {
					expectSQLState(t, err, "23505")
				}
			case <-ctx.Done():
				t.Fatal("concurrent activation timed out")
			}
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM advances WHERE project_id = $1 AND status = 'ACTIVE'", projectID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if successes != 1 || count != 1 {
			t.Fatalf("successful activations = %d; active advances = %d", successes, count)
		}
	})

	t.Run("foreign keys preserve project and actor references", func(t *testing.T) {
		projectID := newProject()
		_, err := pool.Exec(ctx, insertAdvance, 0, "100.00", "DRAFT", actorID)
		expectSQLState(t, err, "23503")
		_, err = pool.Exec(ctx, insertAdvance, projectID, "100.00", "DRAFT", 0)
		expectSQLState(t, err, "23503")
	})

	t.Run("workflow history stores actor and status evidence", func(t *testing.T) {
		var advanceID int64
		if err := pool.QueryRow(ctx, insertAdvance, newProject(), "100.05", "DRAFT", actorID).Scan(&advanceID); err != nil {
			t.Fatal(err)
		}
		const insertEvent = `INSERT INTO advance_workflow_events
			(advance_id, actor_user_id, action, previous_status, resulting_status, after_data)
			VALUES ($1, $2, 'CREATE', NULL, 'DRAFT', '{"amount":"100.05","status":"DRAFT"}')`
		if _, err := pool.Exec(ctx, insertEvent, advanceID, actorID); err != nil {
			t.Fatal(err)
		}
		var storedActor int64
		var status, amount string
		if err := pool.QueryRow(ctx, `SELECT actor_user_id, resulting_status, after_data->>'amount'
			FROM advance_workflow_events WHERE advance_id = $1`, advanceID).Scan(&storedActor, &status, &amount); err != nil {
			t.Fatal(err)
		}
		if storedActor != actorID || status != "DRAFT" || amount != "100.05" {
			t.Fatalf("stored event = %d, %q, %q", storedActor, status, amount)
		}
		_, err := pool.Exec(ctx, insertEvent, 0, actorID)
		expectSQLState(t, err, "23503")
		_, err = pool.Exec(ctx, insertEvent, advanceID, 0)
		expectSQLState(t, err, "23503")
	})

	t.Run("down and up preserve the baseline schema", func(t *testing.T) {
		if err := applySQL(downSQL); err != nil {
			t.Fatalf("roll back advance workflow migration: %v", err)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM projects").Scan(&count); err != nil || count == 0 {
			t.Fatalf("baseline projects after down: count=%d, error=%v", count, err)
		}
		var advancesTable, eventsTable *string
		if err := pool.QueryRow(ctx, "SELECT to_regclass('advances')::text, to_regclass('advance_workflow_events')::text").Scan(&advancesTable, &eventsTable); err != nil {
			t.Fatal(err)
		}
		if advancesTable != nil || eventsTable != nil {
			t.Fatal("advance workflow tables remain after down")
		}
		if err := applySQL(upSQL); err != nil {
			t.Fatalf("reapply advance workflow migration: %v", err)
		}
		projectID := newProject()
		if _, err := pool.Exec(ctx, insertAdvance, projectID, "100.00", "ACTIVE", actorID); err != nil {
			t.Fatal(err)
		}
		_, err := pool.Exec(ctx, insertAdvance, projectID, "100.00", "ACTIVE", actorID)
		expectSQLState(t, err, "23505")
	})
}
