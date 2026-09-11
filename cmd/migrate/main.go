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
	versions, err := migrationVersions()
	if err != nil {
		return err
	}
	sort.Strings(versions)

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, version := range versions {
		if applied[version] {
			fmt.Printf("  skip  %s (already applied)\n", version)
			continue
		}

		upFile := fmt.Sprintf("migrations/%s.up.sql", version)
		content, err := os.ReadFile(upFile)
		if err != nil {
			return fmt.Errorf("read %s: %w", upFile, err)
		}

		upSQL := strings.TrimSpace(string(content))
		if upSQL == "" {
			fmt.Printf("  skip  %s (empty up migration)\n", version)
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
	versions, err := migrationVersions()
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, version := range versions {
		if !applied[version] {
			continue
		}

		downFile := fmt.Sprintf("migrations/%s.down.sql", version)
		content, err := os.ReadFile(downFile)
		if err != nil {
			return fmt.Errorf("read %s: %w", downFile, err)
		}

		downSQL := strings.TrimSpace(string(content))
		if downSQL == "" {
			fmt.Printf("  skip  %s (empty down migration)\n", version)
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

// migrationVersions returns all unique migration versions found in the migrations directory.
// It looks for .up.sql files and extracts the version prefix.
func migrationVersions() ([]string, error) {
	files, err := filepath.Glob("migrations/*.up.sql")
	if err != nil {
		return nil, err
	}

	versions := make([]string, 0, len(files))
	for _, f := range files {
		base := filepath.Base(f)
		version := strings.TrimSuffix(base, ".up.sql")
		versions = append(versions, version)
	}
	return versions, nil
}
