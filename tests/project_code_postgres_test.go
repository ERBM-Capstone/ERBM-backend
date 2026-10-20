package tests

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"gorm.io/gorm"
)

func TestProjectCodePostgresYearRollover(t *testing.T) {
	db, ctx, _, _, _ := advanceCreationDatabase(t)
	repo := repository.NewProjectRepository(db)
	allocate := func(year int, want int64) {
		t.Helper()
		if err := repo.WithTx(ctx, func(tx *gorm.DB) error {
			number, err := repo.NextProjectNumber(ctx, tx, year)
			if err != nil || number != want {
				t.Fatalf("year=%d number=%d error=%v want=%d", year, number, err, want)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	allocate(2026, 1)
	allocate(2026, 2)
	allocate(2027, 1)
	allocate(2026, 3)
	rollback := errors.New("rollback allocation")
	err := repo.WithTx(ctx, func(tx *gorm.DB) error {
		if _, err := repo.NextProjectNumber(ctx, tx, 2027); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	allocate(2027, 2)
}

func TestProjectCreatePostgresConcurrentCodes(t *testing.T) {
	db, ctx, pc, projectID, _ := advanceCreationDatabase(t)
	var pm int64
	if err := db.Raw("SELECT pm_user_id FROM projects WHERE id=?", projectID).Scan(&pm).Error; err != nil {
		t.Fatal(err)
	}
	svc := service.NewProjectService(repository.NewProjectRepository(db))
	type result struct {
		detail *model.ProjectDetail
		err    error
	}
	const requests = 12
	results, start := make(chan result, requests), make(chan struct{})
	for i := 0; i < requests; i++ {
		go func() {
			<-start
			detail, err := svc.Create(ctx, pc, "PC", &dto.CreateProjectRequest{Name: "Concurrent Project", Budget: "1000", PMUserID: &pm})
			results <- result{detail, err}
		}()
	}
	close(start)
	codes, ids := make(map[string]bool), make(map[int64]bool)
	yearCounts := make(map[int]int)
	var first *model.ProjectDetail
	for i := 0; i < requests; i++ {
		var created result
		select {
		case created = <-results:
		case <-ctx.Done():
			t.Fatal("concurrent Project creation timed out")
		}
		if created.err != nil || created.detail == nil {
			t.Fatalf("detail=%v error=%v", created.detail, created.err)
		}
		project := created.detail.Project
		year := project.CreatedAt.In(time.FixedZone("UTC+7", 7*60*60)).Year()
		prefix := fmt.Sprintf("PRJ-%d-", year)
		number, err := strconv.ParseInt(strings.TrimPrefix(project.ProjectCode, prefix), 10, 64)
		if !strings.HasPrefix(project.ProjectCode, prefix) || err != nil || number < 1 || number > requests || codes[project.ProjectCode] || ids[project.ID] {
			t.Fatalf("invalid or duplicate generated code: %#v error=%v", project, err)
		}
		codes[project.ProjectCode], ids[project.ID] = true, true
		yearCounts[year]++
		stored, err := repository.NewProjectRepository(db).GetByID(ctx, project.ID)
		if err != nil || stored.ProjectCode != project.ProjectCode || stored.PCUserID == nil || *stored.PCUserID != pc {
			t.Fatalf("persisted Project=%#v error=%v", stored, err)
		}
		first = created.detail
	}
	for year, count := range yearCounts {
		for number := 1; number <= count; number++ {
			if code := fmt.Sprintf("PRJ-%d-%06d", year, number); !codes[code] {
				t.Errorf("missing generated code %s", code)
			}
		}
		var last int64
		if err := db.Raw("SELECT last_number FROM project_code_counters WHERE year=?", year).Scan(&last).Error; err != nil || last != int64(count) {
			t.Fatalf("year=%d counter=%d want=%d error=%v", year, last, count, err)
		}
	}
	advance, err := service.NewAdvanceService(repository.NewAdvanceRepository(db)).Create(ctx, pc, "PC", &dto.CreateAdvanceRequest{
		ProjectID: first.Project.ID, Purpose: "Generated Project code", Amount: "100",
	})
	if err != nil || advance == nil || advance.AdvanceCode != "ADV-"+first.Project.ProjectCode+"-001" || advance.ProjectCode != first.Project.ProjectCode {
		t.Fatalf("Advance must use the generated Project code: %#v error=%v", advance, err)
	}
}

type failingProjectInsertRepo struct{ *repository.ProjectRepository }

func (r *failingProjectInsertRepo) Create(ctx context.Context, tx *gorm.DB, project *model.Project) error {
	if err := r.ProjectRepository.Create(ctx, tx, project); err != nil {
		return err
	}
	return errors.New("creation failed after insertion")
}

func TestProjectCreatePostgresRollsBackCodeAndProject(t *testing.T) {
	db, ctx, _, projectID, _ := advanceCreationDatabase(t)
	var pm int64
	if err := db.Raw("SELECT pm_user_id FROM projects WHERE id=?", projectID).Scan(&pm).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewProjectRepository(db)
	req := &dto.CreateProjectRequest{Name: "Rollback Project", Budget: "1000"}
	detail, err := service.NewProjectService(&failingProjectInsertRepo{repo}).Create(ctx, pm, "PM", req)
	if !errors.Is(err, dto.ErrInternalServer) || detail != nil {
		t.Fatalf("detail=%v error=%v", detail, err)
	}
	var projects, counters int64
	if err := db.Model(&model.Project{}).Where("name = ?", "Rollback Project").Count(&projects).Error; err != nil || projects != 0 {
		t.Fatalf("rolled back Projects=%d error=%v", projects, err)
	}
	if err := db.Table("project_code_counters").Count(&counters).Error; err != nil || counters != 0 {
		t.Fatalf("rolled back counters=%d error=%v", counters, err)
	}
	detail, err = service.NewProjectService(repo).Create(ctx, pm, "PM", req)
	if err != nil || detail == nil || !strings.HasSuffix(detail.Project.ProjectCode, "-000001") {
		t.Fatalf("first committed Project=%#v error=%v", detail, err)
	}
}
