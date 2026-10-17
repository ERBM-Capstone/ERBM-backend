package tests

import (
	"context"
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

func TestProjectUpdatePostgresWithOneConnection(t *testing.T) {
	db, ctx, pc, projectID, otherProjectID := advanceCreationDatabase(t)
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	projects := service.NewProjectService(repository.NewProjectRepository(db))
	name := "PC edit with one connection"
	detail, err := projects.Update(ctx, pc, "PC", projectID, &dto.UpdateProjectRequest{Name: &name})
	if err != nil || detail.Project.Name != name {
		t.Fatalf("PC update=%v error=%v", detail, err)
	}
	pm := detail.Project.PMUserID
	if _, err := projects.Update(ctx, pm, "PM", projectID, &dto.UpdateProjectRequest{PMUserID: &pm, PCUserID: &pc}); err != nil {
		t.Fatalf("assignment lookup must use the held connection: %v", err)
	}
	advances := service.NewAdvanceService(repository.NewAdvanceRepository(db))
	create := func(id int64) *model.AdvanceListItem {
		t.Helper()
		advance, err := advances.Create(ctx, pc, "PC", &dto.CreateAdvanceRequest{ProjectID: id, Purpose: "Closure fixture", Amount: "100"})
		if err != nil {
			t.Fatal(err)
		}
		return advance
	}
	create(otherProjectID) // This Project's DRAFT must not block projectID.
	advance := create(projectID)
	closed, newName := "CLOSED", "Must not be written while an Advance is open"
	for _, status := range []model.AdvanceStatus{
		model.AdvanceStatusDraft, model.AdvanceStatusPendingPMApproval, model.AdvanceStatusReturnedForRevision,
		model.AdvanceStatusApproved, model.AdvanceStatusActive,
	} {
		if err := db.WithContext(ctx).Exec("UPDATE advances SET status=? WHERE id=?", status, advance.ID).Error; err != nil {
			t.Fatal(err)
		}
		_, err := projects.Update(ctx, pm, "PM", projectID, &dto.UpdateProjectRequest{Name: &newName, Status: &closed})
		if !errors.Is(err, dto.ErrProjectHasUnclosedAdvances) {
			t.Fatalf("status=%s close error=%v", status, err)
		}
		var stored model.Project
		if err := db.WithContext(ctx).First(&stored, projectID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Name != name || stored.Status != model.ProjectStatusActive || stored.ClosedAt != nil || stored.ClosedBy != nil {
			t.Fatalf("rejected close changed persisted data: %#v", stored)
		}
	}
	if err := db.WithContext(ctx).Exec("UPDATE advances SET status='CLOSED' WHERE id=?", advance.ID).Error; err != nil {
		t.Fatal(err)
	}
	detail, err = projects.Update(ctx, pm, "PM", projectID, &dto.UpdateProjectRequest{Status: &closed})
	if err != nil || detail.Project.Status != model.ProjectStatusClosed || detail.Project.ClosedAt == nil || detail.Project.ClosedBy == nil || *detail.Project.ClosedBy != pm {
		t.Fatalf("final close=%v error=%v", detail, err)
	}
}

type projectOperationGate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newProjectOperationGate(t *testing.T) *projectOperationGate {
	t.Helper()
	gate := &projectOperationGate{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(gate.open)
	return gate
}

func (g *projectOperationGate) wait(ctx context.Context) error {
	close(g.entered)
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *projectOperationGate) open() { g.once.Do(func() { close(g.release) }) }

type pausedProjectCloseRepo struct {
	*repository.ProjectRepository
	gate *projectOperationGate
}

func (r *pausedProjectCloseRepo) HasUnclosedAdvances(ctx context.Context, tx *gorm.DB, id int64) (bool, error) {
	unclosed, err := r.ProjectRepository.HasUnclosedAdvances(ctx, tx, id)
	if err == nil {
		err = r.gate.wait(ctx)
	}
	return unclosed, err
}

type pausedAdvanceCreateRepo struct {
	*repository.AdvanceRepository
	gate *projectOperationGate
}

func (r *pausedAdvanceCreateRepo) Create(ctx context.Context, tx *gorm.DB, advance *model.Advance) error {
	if err := r.gate.wait(ctx); err != nil {
		return err
	}
	return r.AdvanceRepository.Create(ctx, tx, advance)
}

func TestProjectClosePostgresSerializesWithAdvanceCreation(t *testing.T) {
	for _, closeFirst := range []bool{true, false} {
		name := "creation holds the Project lock first"
		if closeFirst {
			name = "closing holds the Project lock first"
		}
		t.Run(name, func(t *testing.T) {
			db, ctx, pc, projectID, _ := advanceCreationDatabase(t)
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			var pm int64
			if err := db.WithContext(ctx).Raw("SELECT pm_user_id FROM projects WHERE id=?", projectID).Scan(&pm).Error; err != nil {
				t.Fatal(err)
			}
			gate := newProjectOperationGate(t)
			projectRepo := repository.NewProjectRepository(db)
			advanceRepo := repository.NewAdvanceRepository(db)
			projects := service.NewProjectService(projectRepo)
			advances := service.NewAdvanceService(advanceRepo)
			if closeFirst {
				projects = service.NewProjectService(&pausedProjectCloseRepo{projectRepo, gate})
			} else {
				advances = service.NewAdvanceService(&pausedAdvanceCreateRepo{advanceRepo, gate})
			}
			closeResult, createResult := make(chan error, 1), make(chan error, 1)
			closeProject := func() {
				closed := "CLOSED"
				_, err := projects.Update(ctx, pm, "PM", projectID, &dto.UpdateProjectRequest{Status: &closed})
				closeResult <- err
			}
			createAdvance := func() {
				_, err := advances.Create(ctx, pc, "PC", &dto.CreateAdvanceRequest{ProjectID: projectID, Purpose: "Concurrent creation", Amount: "100"})
				createResult <- err
			}
			first, second, secondResult := closeProject, createAdvance, createResult
			if !closeFirst {
				first, second, secondResult = createAdvance, closeProject, closeResult
			}
			go first()
			select {
			case <-gate.entered:
			case <-ctx.Done():
				t.Fatal("first operation did not reach its locked critical section")
			}
			go second()
			select {
			case err := <-secondResult:
				t.Fatalf("second operation bypassed the held Project lock: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			gate.open()
			closeErr, createErr := <-closeResult, <-createResult
			var project model.Project
			var count int64
			if err := db.WithContext(ctx).First(&project, projectID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.WithContext(ctx).Model(&model.Advance{}).Where("project_id=?", projectID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if closeFirst {
				if closeErr != nil || !errors.Is(createErr, dto.ErrProjectClosed) || project.Status != model.ProjectStatusClosed || count != 0 {
					t.Fatalf("closing first: close=%v create=%v status=%s advances=%d", closeErr, createErr, project.Status, count)
				}
			} else if createErr != nil || !errors.Is(closeErr, dto.ErrProjectHasUnclosedAdvances) || project.Status != model.ProjectStatusActive || count != 1 {
				t.Fatalf("creation first: close=%v create=%v status=%s advances=%d", closeErr, createErr, project.Status, count)
			}
		})
	}
}
