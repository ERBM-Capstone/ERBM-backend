package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down>")
	}
	direction := os.Args[1]
	if direction != "up" && direction != "down" {
		log.Fatalf("unknown direction %q — use 'up' or 'down'", direction)
	}

	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file, reading from environment")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// Ensure migrations table exists
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		log.Fatalf("ensure table: %v", err)
	}

	switch direction {
	case "up":
		if err := migrateUp(ctx, pool); err != nil {
			log.Fatalf("migrate up: %v", err)
		}
	case "down":
		if err := migrateDown(ctx, pool); err != nil {
			log.Fatalf("migrate down: %v", err)
		}
	}
}

// ensureMigrationsTable creates the schema_migrations table if it does not exist.
func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT        NOT NULL PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	return err
}

// appliedVersions returns the set of already-applied migration versions.
func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// migrateUp runs every pending migration file in ascending order.
func migrateUp(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := sqlFiles()
	if err != nil {
		return err
	}
	sort.Strings(files)

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, f := range files {
		version := migrationVersion(f)
		if applied[version] {
			fmt.Printf("  skip  %s (already applied)\n", version)
			continue
		}

		content, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		upSQL := extractSection(string(content), "up")
		if strings.TrimSpace(upSQL) == "" {
			fmt.Printf("  skip  %s (empty up section)\n", version)
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		if _, err := tx.Exec(ctx, upSQL); err != nil {
			return fmt.Errorf("apply %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return fmt.Errorf("record %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		fmt.Printf("  apply %s\n", version)
	}
	fmt.Println("migrate up: done")
	return nil
}

// migrateDown rolls back the most recently applied migration.
func migrateDown(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := sqlFiles()
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, f := range files {
		version := migrationVersion(f)
		if !applied[version] {
			continue
		}

		content, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		downSQL := extractSection(string(content), "down")
		if strings.TrimSpace(downSQL) == "" {
			fmt.Printf("  skip  %s (empty down section)\n", version)
			return nil
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		if _, err := tx.Exec(ctx, downSQL); err != nil {
			return fmt.Errorf("rollback %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
			return fmt.Errorf("unrecord %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		fmt.Printf("  rollback %s\n", version)
		return nil // only one migration at a time
	}
	fmt.Println("migrate down: nothing to roll back")
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func sqlFiles() ([]string, error) {
	return filepath.Glob("migrations/*.sql")
}

func migrationVersion(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".sql")
}

// extractSection parses -- migrate:up / -- migrate:down markers.
func extractSection(content, section string) string {
	marker := fmt.Sprintf("-- migrate:%s", section)
	idx := strings.Index(content, marker)
	if idx == -1 {
		return ""
	}
	body := content[idx+len(marker):]

	// Stop at the next marker
	var otherMarker string
	if section == "up" {
		otherMarker = "-- migrate:down"
	} else {
		otherMarker = "-- migrate:up"
	}
	if end := strings.Index(body, otherMarker); end != -1 {
		body = body[:end]
	}
	return strings.TrimSpace(body)
}
