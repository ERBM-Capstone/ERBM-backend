package tests

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func otherProjectCoordinator(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var employeeID, userID int64
	if err := db.Raw(`INSERT INTO employees(first_name,last_name,position,email,date_hired) VALUES ('Other','Coordinator','PC','other-coordinator@example.test','2026-01-01') RETURNING id`).Scan(&employeeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`INSERT INTO users(employee_id,email,role_id,status) SELECT ?,'other-coordinator@example.test',id,'ACTIVE' FROM roles WHERE code='PC' RETURNING id`, employeeID).Scan(&userID).Error; err != nil {
		t.Fatal(err)
	}
	return userID
}

func requireCoordinatorSQLState(t *testing.T, err error, state string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != state {
		t.Fatalf("SQL state: want %s, got %v", state, err)
	}
}

func TestProjectCoordinatorForeignKeyMigration(t *testing.T) {
	db, _, actor, one, _ := advanceCreationDatabase(t)
	var pmID int64
	if err := db.Raw(`SELECT pm_user_id FROM projects WHERE id=?`, one).Scan(&pmID).Error; err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 2; cycle++ {
		var count int64
		if err := db.Raw(`SELECT COUNT(*) FROM projects WHERE pc_user_id=? AND pm_user_id=?`, actor, pmID).Scan(&count).Error; err != nil || count != 2 {
			t.Fatalf("shared PC and PM assignments=%d, %v", count, err)
		}
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='projects' AND column_name='pc_user_id' AND data_type='bigint' AND is_nullable='YES'`).Scan(&count).Error; err != nil || count != 1 {
			t.Fatalf("nullable PC foreign key column=%d, %v", count, err)
		}
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='project_coordinators'`).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("obsolete coordinator table=%d, %v", count, err)
		}
		if err := db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname=current_schema() AND tablename='projects' AND indexname='idx_projects_pc_user_id'`).Scan(&count).Error; err != nil || count != 1 {
			t.Fatalf("PC lookup index=%d, %v", count, err)
		}
		var unassigned int64
		if err := db.Raw(`INSERT INTO projects(project_code,name,budget,pm_user_id) VALUES ('UNASSIGNED','Optional PC',1000,?) RETURNING id`, pmID).Scan(&unassigned).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Raw(`SELECT COUNT(*) FROM projects WHERE id=? AND pc_user_id IS NULL`, unassigned).Scan(&count).Error; err != nil || count != 1 {
			t.Fatalf("unassigned Project=%d, %v", count, err)
		}
		requireCoordinatorSQLState(t, db.Exec(`INSERT INTO projects(project_code,name,budget,pm_user_id) VALUES ('NO_PM','Missing PM',1000,NULL)`).Error, "23502")
		requireCoordinatorSQLState(t, db.Exec(`UPDATE projects SET pc_user_id=-1 WHERE id=?`, one).Error, "23503")
		requireCoordinatorSQLState(t, db.Exec(`UPDATE projects SET pm_user_id=-1 WHERE id=?`, one).Error, "23503")
		requireCoordinatorSQLState(t, db.Exec(`DELETE FROM users WHERE id=?`, actor).Error, "23503")
		requireCoordinatorSQLState(t, db.Exec(`DELETE FROM users WHERE id=?`, pmID).Error, "23503")
		if err := db.Raw(`SELECT COUNT(*) FROM projects WHERE id=? AND pc_user_id=? AND pm_user_id=?`, one, actor, pmID).Scan(&count).Error; err != nil || count != 1 {
			t.Fatalf("failed writes changed assignment=%d, %v", count, err)
		}
		if cycle == 1 {
			break
		}
		// Remove the dependent Advance schema before the Project round trip.
		advanceApplyMigration(t, db, "0005_create_advance_workflow.down.sql")
		advanceApplyMigration(t, db, "0003_create_projects.down.sql")
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('projects','project_coordinators','project_code_counters')`).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("Project tables after down=%d, %v", count, err)
		}
		advanceApplyMigration(t, db, "0003_create_projects.up.sql")
		for _, code := range []string{"RECREATED_ONE", "RECREATED_TWO"} {
			var id int64
			if err := db.Raw(`INSERT INTO projects(project_code,name,budget,pm_user_id,pc_user_id) VALUES (?, 'Round trip',1000,?,?) RETURNING id`, code, pmID, actor).Scan(&id).Error; err != nil {
				t.Fatal(err)
			}
			one = id
		}
	}
}

func TestProjectRepositoryDirectCoordinatorMembership(t *testing.T) {
	db, ctx, actor, one, two := advanceCreationDatabase(t)
	other := otherProjectCoordinator(t, db)
	var pmID, unassigned int64
	if err := db.Raw(`SELECT pm_user_id FROM projects WHERE id=?`, one).Scan(&pmID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`INSERT INTO projects(project_code,name,budget,pm_user_id) VALUES ('UNASSIGNED','Optional PC',1000,?) RETURNING id`, pmID).Scan(&unassigned).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE projects SET status='CLOSED' WHERE id=?`, two).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewProjectRepository(db)
	for _, tt := range []struct {
		name   string
		filter model.ProjectFilter
		ids    []int64
		total  int64
	}{
		{"PC projects", model.ProjectFilter{MemberUserID: &actor, Page: 1, PageSize: 20}, []int64{two, one}, 2},
		{"PC pagination", model.ProjectFilter{MemberUserID: &actor, Page: 2, PageSize: 1}, []int64{one}, 2},
		{"PC active projects", model.ProjectFilter{MemberUserID: &actor, Status: "ACTIVE", Page: 1, PageSize: 20}, []int64{one}, 1},
		{"PM projects", model.ProjectFilter{MemberUserID: &pmID, Page: 1, PageSize: 20}, []int64{unassigned, two, one}, 3},
		{"PM active search", model.ProjectFilter{MemberUserID: &pmID, Status: "ACTIVE", Query: "PRJ001", Page: 1, PageSize: 20}, []int64{one}, 1},
		{"unassigned PC", model.ProjectFilter{MemberUserID: &other, Page: 1, PageSize: 20}, nil, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			projects, total, err := repo.List(ctx, tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			var ids []int64
			for _, project := range projects {
				ids = append(ids, project.ID)
				if project.ID == unassigned {
					if project.PCUserID != nil {
						t.Fatalf("unassigned PC=%v", project.PCUserID)
					}
				} else if project.PCUserID == nil || *project.PCUserID != actor {
					t.Fatalf("assigned Project=%#v", project)
				}
			}
			if total != tt.total || !reflect.DeepEqual(ids, tt.ids) {
				t.Fatalf("ids=%v, total=%d; want %v, %d", ids, total, tt.ids, tt.total)
			}
		})
	}
	for _, tt := range []struct {
		project, user int64
		want          bool
	}{{one, actor, true}, {two, actor, true}, {one, other, false}, {one, pmID, false}, {unassigned, actor, false}, {-1, actor, false}} {
		assigned, err := repo.IsCoordinator(ctx, tt.project, tt.user)
		if err != nil || assigned != tt.want {
			t.Fatalf("membership (%d,%d)=%v, %v", tt.project, tt.user, assigned, err)
		}
	}
	projects, total, err := service.NewProjectService(repo).List(ctx, pmID, "PM", model.ProjectFilter{Page: 1, PageSize: 20})
	if err != nil || total != 3 || len(projects) != 3 {
		t.Fatalf("project details=%#v, %d, %v", projects, total, err)
	}
	for _, detail := range projects {
		if detail.PM == nil || detail.PM.UserID != pmID {
			t.Fatalf("manager=%#v", detail.PM)
		}
		if detail.Project.ID == unassigned {
			if detail.PC != nil {
				t.Fatalf("unassigned Project has PC=%#v", detail.PC)
			}
		} else if detail.PC == nil || detail.PC.UserID != actor || detail.PC.FirstName != "Somchai" || detail.PC.LastName != "Jaidee" || detail.PC.Email != "create@example.test" {
			t.Fatalf("coordinator=%#v", detail.PC)
		}
	}
	for _, ids := range [][]int64{nil, {-1}} {
		members, err := repo.GetMembersByUserIDs(ctx, ids)
		if err != nil || len(members) != 0 {
			t.Fatalf("missing member profiles=%#v, %v", members, err)
		}
	}
}

func TestProjectMemberProfilesRemainAvailableAfterReassignment(t *testing.T) {
	db, ctx, actor, one, two := advanceCreationDatabase(t)
	other := otherProjectCoordinator(t, db)
	repo := repository.NewProjectRepository(db)
	loaded, err := repo.GetByID(ctx, one)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE projects SET pc_user_id=? WHERE id IN ?", other, []int64{one, two}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE users SET status='DEACTIVATED' WHERE id=?", actor).Error; err != nil {
		t.Fatal(err)
	}
	// The already-read assignment must still resolve to its original user's
	// profile, even though that user is no longer assigned or ACTIVE.
	members, err := repo.GetMembersByUserIDs(ctx, []int64{loaded.PMUserID, *loaded.PCUserID, other, actor, -1})
	if err != nil || len(members) != 3 {
		t.Fatalf("profiles=%#v error=%v", members, err)
	}
	want := map[int64]model.ProjectUserSummary{
		loaded.PMUserID: {UserID: loaded.PMUserID, FirstName: "Manager", LastName: "Fixture", Email: "manager-create@example.test"},
		actor:           {UserID: actor, FirstName: "Somchai", LastName: "Jaidee", Email: "create@example.test"},
		other:           {UserID: other, FirstName: "Other", LastName: "Coordinator", Email: "other-coordinator@example.test"},
	}
	for _, member := range members {
		expected, ok := want[member.UserID]
		if !ok || member != expected {
			t.Fatalf("unexpected profile=%#v", member)
		}
		delete(want, member.UserID)
	}
	if len(want) != 0 {
		t.Fatalf("missing profiles=%#v", want)
	}
}

func TestDirectProjectCoordinatorControlsAdvanceAccessAfterReassignmentAndRemoval(t *testing.T) {
	db, ctx, actor, one, _ := advanceCreationDatabase(t)
	other := otherProjectCoordinator(t, db)
	advanceRepo := repository.NewAdvanceRepository(db)
	advances := service.NewAdvanceService(advanceRepo)
	req := &dto.CreateAdvanceRequest{ProjectID: one, Purpose: "Travel", Amount: "100"}
	first, err := advances.Create(ctx, actor, "PC", req)
	if err != nil || first.AdvanceCode != "ADV-PRJ001-001" {
		t.Fatalf("assigned creator=%#v, %v", first, err)
	}
	if err := db.Exec(`UPDATE advances SET status='PENDING_PM_APPROVAL' WHERE id=?`, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if items, total, err := advances.List(ctx, actor, "PC", model.AdvanceFilter{ProjectID: &one, Page: 1, PageSize: 20}); err != nil || total != 1 || len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("assigned list=%#v, %d, %v", items, total, err)
	}
	if item, err := advances.Create(ctx, other, "PC", req); !errors.Is(err, dto.ErrForbidden) || item != nil {
		t.Fatalf("unassigned creator=%#v, %v", item, err)
	}
	var pmID int64
	if err := db.Raw(`SELECT pm_user_id FROM projects WHERE id=?`, one).Scan(&pmID).Error; err != nil {
		t.Fatal(err)
	}
	projects := service.NewProjectService(repository.NewProjectRepository(db))
	if detail, err := projects.Update(ctx, pmID, "PM", one, &dto.UpdateProjectRequest{PCUserID: &other}); err != nil || detail.PC == nil || detail.PC.UserID != other || detail.Project.PCUserID == nil || *detail.Project.PCUserID != other {
		t.Fatalf("reassigned Project=%#v, %v", detail, err)
	}
	if item, err := advances.Create(ctx, actor, "PC", req); !errors.Is(err, dto.ErrForbidden) || item != nil {
		t.Fatalf("former creator=%#v, %v", item, err)
	}
	for _, tt := range []struct {
		user int64
		want bool
	}{{actor, false}, {other, true}} {
		access, err := advanceRepo.GetProjectAccess(ctx, one, tt.user)
		if err != nil || access.IsCoordinator != tt.want {
			t.Fatalf("reassigned access=%#v, %v", access, err)
		}
	}
	second, err := advances.Create(ctx, other, "PC", req)
	if err != nil || second.AdvanceCode != "ADV-PRJ001-002" || second.CreatedBy != other {
		t.Fatalf("new creator=%#v, %v", second, err)
	}
	if items, total, err := advances.List(ctx, other, "PC", model.AdvanceFilter{ProjectID: &one, Page: 1, PageSize: 20}); err != nil || total != 2 || len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("replacement list=%#v, %d, %v", items, total, err)
	}
	if items, total, err := advances.List(ctx, actor, "PC", model.AdvanceFilter{ProjectID: &one, Page: 1, PageSize: 20}); !errors.Is(err, dto.ErrForbidden) || total != 0 || len(items) != 0 {
		t.Fatalf("former PC scoped list=%#v, %d, %v", items, total, err)
	}
	if items, total, err := advances.List(ctx, actor, "PC", model.AdvanceFilter{Page: 1, PageSize: 20}); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("former PC global list=%#v, %d, %v", items, total, err)
	}
	var remove dto.UpdateProjectRequest
	if err := json.Unmarshal([]byte(`{"pc_user_id":null}`), &remove); err != nil {
		t.Fatal(err)
	}
	if detail, err := projects.Update(ctx, pmID, "PM", one, &remove); err != nil || detail.PC != nil || detail.Project.PCUserID != nil {
		t.Fatalf("unassigned Project=%#v, %v", detail, err)
	}
	for _, user := range []int64{actor, other} {
		if item, err := advances.Create(ctx, user, "PC", req); !errors.Is(err, dto.ErrForbidden) || item != nil {
			t.Fatalf("removed creator=%#v, %v", item, err)
		}
		if items, total, err := advances.List(ctx, user, "PC", model.AdvanceFilter{ProjectID: &one, Page: 1, PageSize: 20}); !errors.Is(err, dto.ErrForbidden) || total != 0 || len(items) != 0 {
			t.Fatalf("removed PC scoped list=%#v, %d, %v", items, total, err)
		}
		if items, total, err := advances.List(ctx, user, "PC", model.AdvanceFilter{Page: 1, PageSize: 20}); err != nil || total != 0 || len(items) != 0 {
			t.Fatalf("removed PC global list=%#v, %d, %v", items, total, err)
		}
	}
	var records, events, counter int64
	if err := db.Raw(`SELECT COUNT(*) FROM advances WHERE project_id=?`, one).Scan(&records).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT COUNT(*) FROM advance_workflow_events e JOIN advances a ON a.id=e.advance_id WHERE a.project_id=?`, one).Scan(&events).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT last_number FROM project_advance_counters WHERE project_id=?`, one).Scan(&counter).Error; err != nil {
		t.Fatal(err)
	}
	if records != 2 || events != 2 || counter != 2 {
		t.Fatalf("unauthorized requests wrote records/events/counter=%d/%d/%d", records, events, counter)
	}
}
