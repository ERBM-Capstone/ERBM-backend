package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type advanceCapturedQuery struct {
	sql  string
	args []any
}

func advanceDryRunRepository(t *testing.T) (*repository.AdvanceRepository, *[]advanceCapturedQuery) {
	t.Helper()
	db, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	var queries []advanceCapturedQuery
	if err := db.Callback().Query().After("gorm:query").Register("capture_advance_query", func(db *gorm.DB) {
		queries = append(queries, advanceCapturedQuery{db.Statement.SQL.String(), append([]any(nil), db.Statement.Vars...)})
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	return repository.NewAdvanceRepository(db), &queries
}

func TestAdvanceRepositoryAppliesVisibleScopeBeforeCountAndPaging(t *testing.T) {
	for _, role := range []string{"PC", "PM", "ADMIN", "ACCOUNTING"} {
		t.Run(role, func(t *testing.T) {
			repo, queries := advanceDryRunRepository(t)
			projectID := int64(10)
			_, _, err := repo.List(context.Background(), model.AdvanceFilter{
				ActorUserID: 25, ActorRole: role, ProjectID: &projectID,
				Status: model.AdvanceStatusDraft, Query: "a' OR 1=1 --", Page: 2, PageSize: 10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(*queries) != 2 {
				t.Fatalf("queries = %#v", *queries)
			}
			for _, query := range *queries {
				if !strings.Contains(query.sql, "a.status <>") || !strings.Contains(query.sql, "a.created_by =") || !strings.Contains(query.sql, "a.project_id =") || !strings.Contains(query.sql, "a.purpose ILIKE") || strings.Contains(query.sql, "a' OR 1=1") {
					t.Fatalf("missing visible/bound scope in %s", query.sql)
				}
				if role == "PC" && !strings.Contains(query.sql, "p.pc_user_id =") {
					t.Fatal("PC scope must use Project membership")
				}
				if role == "PM" && !strings.Contains(query.sql, "p.pm_user_id =") {
					t.Fatal("PM scope must use assigned manager")
				}
				foundActor, foundSearch := false, false
				for _, arg := range query.args {
					if actor, ok := arg.(int64); ok && actor == 25 {
						foundActor = true
					}
					if search, ok := arg.(string); ok && search == "%a' OR 1=1 --%" {
						foundSearch = true
					}
				}
				if !foundActor || !foundSearch {
					t.Fatalf("bound args = %#v", query.args)
				}
			}
			if !strings.Contains((*queries)[0].sql, "count(*)") || strings.Contains((*queries)[0].sql, "LIMIT") || strings.Contains((*queries)[0].sql, "OFFSET") {
				t.Fatal("count must precede pagination")
			}
			if !strings.Contains((*queries)[1].sql, "a.created_at DESC, a.id DESC") || !strings.Contains((*queries)[1].sql, "LIMIT") || !strings.Contains((*queries)[1].sql, "OFFSET") || !strings.Contains((*queries)[1].sql, "a.amount::text") {
				t.Fatalf("paged monetary query = %s", (*queries)[1].sql)
			}
		})
	}
}

func TestAdvanceRepositoryRejectsMissingOrUnsupportedActor(t *testing.T) {
	for _, tt := range []struct {
		actor int64
		role  string
		want  error
	}{
		{0, "ADMIN", dto.ErrUnauthorized}, {25, "EMPLOYEE", dto.ErrForbidden},
	} {
		repo, queries := advanceDryRunRepository(t)
		_, _, err := repo.List(context.Background(), model.AdvanceFilter{ActorUserID: tt.actor, ActorRole: tt.role, Page: 1, PageSize: 20})
		if !errors.Is(err, tt.want) || len(*queries) != 0 {
			t.Fatalf("error = %v; queries = %#v", err, *queries)
		}
	}
}

func TestAdvanceRepositoryPostgresVisibility(t *testing.T) {
	databaseURL := os.Getenv("ERBM_MIGRATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ERBM_MIGRATION_TEST_DATABASE_URL for PostgreSQL visibility integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	schema := fmt.Sprintf("erbm_advance_list_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid migration test database configuration")
	}
	config.RuntimeParams["search_path"] = quotedSchema
	raw := stdlib.OpenDB(*config)
	t.Cleanup(func() { raw.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: raw}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("could not open the configured PostgreSQL test database")
	}
	db = db.WithContext(ctx)
	if err := db.Exec("CREATE SCHEMA " + quotedSchema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := raw.ExecContext(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Error("could not remove test schema")
		}
	})
	for _, name := range []string{"0001_create_auth_tables.up.sql", "0002_add_employee_user_audit_fields.up.sql", "0003_create_projects.up.sql", "0004_create_account_activation_tokens.up.sql", "0005_create_advance_workflow.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			t.Fatal(err)
		}
	}
	userCounter := 0
	newUser := func(role string) int64 {
		t.Helper()
		userCounter++
		var employeeID, userID int64
		email := fmt.Sprintf("advance-%d@example.test", userCounter)
		if err := raw.QueryRowContext(ctx, `INSERT INTO employees (first_name,last_name,position,email,date_hired) VALUES ('Advance','Fixture',$1,$2,'2026-01-01') RETURNING id`, role, email).Scan(&employeeID); err != nil {
			t.Fatal(err)
		}
		if err := raw.QueryRowContext(ctx, `INSERT INTO users (employee_id,email,role_id,status) SELECT $1,$2,id,'ACTIVE' FROM roles WHERE code=$3 RETURNING id`, employeeID, email, role).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		return userID
	}
	admin, pm, pc, otherPC, accounting, otherPM, foreignPC := newUser("ADMIN"), newUser("PM"), newUser("PC"), newUser("PC"), newUser("ACCOUNTING"), newUser("PM"), newUser("PC")
	newProject := func(code string, manager, coordinator int64) int64 {
		t.Helper()
		var id int64
		if err := raw.QueryRowContext(ctx, `INSERT INTO projects(project_code,name,budget,pm_user_id,pc_user_id) VALUES ($1,$1,10000,$2,$3) RETURNING id`, code, manager, coordinator).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	one, two := newProject("ONE", pm, pc), newProject("TWO", otherPM, foreignPC)
	fixtureNumbers := map[int64]int{}
	insert := func(project, creator int64, status, purpose string) int64 {
		t.Helper()
		fixtureNumbers[project]++
		prefix := "ONE"
		if project == two {
			prefix = "TWO"
		}
		code := fmt.Sprintf("ADV-%s-%03d", prefix, fixtureNumbers[project])
		var id int64
		if err := raw.QueryRowContext(ctx, `INSERT INTO advances(advance_code,project_id,purpose,amount,status,created_by,updated_by,created_at,updated_at) VALUES ($5,$1,$2,'1234.56',$3,$4,$4,'2026-10-15T02:00:00Z','2026-10-15T02:00:00Z') RETURNING id`, project, purpose, status, creator, code).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	ownDraft := insert(one, pc, "DRAFT", "Draft one")
	insert(one, otherPC, "DRAFT", "Former coordinator draft")
	pending := insert(one, otherPC, "PENDING_PM_APPROVAL", "Trip one")
	active := insert(one, pc, "ACTIVE", "Hotel")
	foreignDraft := insert(two, foreignPC, "DRAFT", "Foreign draft")
	approved := insert(two, foreignPC, "APPROVED", "Trip two")
	if err := db.Exec(`INSERT INTO project_advance_counters(project_id,last_number) SELECT project_id,COUNT(*) FROM advances GROUP BY project_id`).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewAdvanceRepository(db)
	for _, tt := range []struct {
		name, role string
		actor      int64
		ids        []int64
	}{
		{"admin", "ADMIN", admin, []int64{approved, active, pending}},
		{"accounting", "ACCOUNTING", accounting, []int64{approved, active, pending}},
		{"manager", "PM", pm, []int64{active, pending}},
		{"other manager", "PM", otherPM, []int64{approved}},
		{"creator", "PC", pc, []int64{active, pending, ownDraft}},
		{"former coordinator without assignment", "PC", otherPC, nil},
		{"foreign coordinator", "PC", foreignPC, []int64{approved, foreignDraft}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items, total, err := repo.List(ctx, model.AdvanceFilter{ActorUserID: tt.actor, ActorRole: tt.role, Page: 1, PageSize: 20})
			if err != nil {
				t.Fatal(err)
			}
			var ids []int64
			for _, item := range items {
				ids = append(ids, item.ID)
				if item.Amount != "1234.56" {
					t.Fatalf("amount = %q", item.Amount)
				}
				if item.AdvanceCode == "" || item.RequesterFirstName != "Advance" || item.RequesterLastName != "Fixture" {
					t.Fatalf("display fields = %#v", item)
				}
			}
			if total != int64(len(tt.ids)) || !reflect.DeepEqual(ids, tt.ids) {
				t.Fatalf("ids=%v, total=%d; want %v", ids, total, tt.ids)
			}
		})
	}
	items, total, err := repo.List(ctx, model.AdvanceFilter{ActorUserID: pc, ActorRole: "PC", Page: 2, PageSize: 1})
	if err != nil || total != 3 || len(items) != 1 || items[0].ID != pending {
		t.Fatalf("page after visibility = %#v, %d, %v", items, total, err)
	}
	items, total, err = repo.List(ctx, model.AdvanceFilter{ActorUserID: admin, ActorRole: "ADMIN", Status: model.AdvanceStatusDraft, Page: 1, PageSize: 20})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("admin drafts = %#v, %d, %v", items, total, err)
	}
	items, total, err = repo.List(ctx, model.AdvanceFilter{ActorUserID: pc, ActorRole: "PC", ProjectID: &two, Page: 1, PageSize: 20})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("foreign project = %#v, %d, %v", items, total, err)
	}
	items, total, err = repo.List(ctx, model.AdvanceFilter{ActorUserID: admin, ActorRole: "ADMIN", Query: "trip", Page: 1, PageSize: 20})
	if err != nil || total != 2 || len(items) != 2 || items[0].ID != approved || items[1].ID != pending {
		t.Fatalf("search = %#v, %d, %v", items, total, err)
	}
	access, err := repo.GetProjectAccess(ctx, one, pc)
	if err != nil || access.ProjectID != one || access.PMUserID != pm || !access.IsCoordinator {
		t.Fatalf("project access = %#v, %v", access, err)
	}
	_, err = repo.GetProjectAccess(ctx, 0, pc)
	if !errors.Is(err, dto.ErrProjectNotFound) {
		t.Fatalf("missing Project = %v", err)
	}
}
