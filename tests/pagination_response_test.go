package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
)

func TestWithMetaPreservesZeroTotalForEmptyLists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name string
		data any
	}{
		{name: "employees", data: []dto.EmployeeResponse{}},
		{name: "projects", data: []dto.ProjectResponse{}},
		{name: "advances", data: []dto.AdvanceListItemResponse{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			dto.WithMeta(ctx, http.StatusOK, tt.data, "OK", &dto.Meta{
				Page: 2, PerPage: 10, Total: 0,
			})
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d; body = %s", recorder.Code, recorder.Body.String())
			}

			var response struct {
				Success bool                       `json:"success"`
				Message string                     `json:"message"`
				Data    json.RawMessage            `json:"data"`
				Meta    map[string]json.RawMessage `json:"meta"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !response.Success || response.Message != "OK" || string(response.Data) != "[]" {
				t.Fatalf("empty list response = %s", recorder.Body.String())
			}
			for key, want := range map[string]string{"page": "2", "per_page": "10", "total": "0"} {
				if got := string(response.Meta[key]); got != want {
					t.Errorf("meta.%s = %q, want %q; body = %s", key, got, want, recorder.Body.String())
				}
			}
		})
	}
}

func TestProjectAndAdvanceServicesPreservePaginationPolicies(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tt := range []struct {
		name                   string
		page, size             int
		projectErr, advanceErr error
		wantPage, wantSize     int
	}{
		{"both zero", 0, 0, nil, dto.ErrBadRequest, 1, 20},
		{"page zero", 0, 10, nil, dto.ErrBadRequest, 1, 10},
		{"size zero", 2, 0, nil, dto.ErrBadRequest, 2, 20},
		{"explicit page", 2, 10, nil, nil, 2, 10},
		{"maximum page size", 1, 100, nil, nil, 1, 100},
		{"negative page", -1, 20, dto.ErrBadRequest, dto.ErrBadRequest, 0, 0},
		{"negative size", 1, -1, dto.ErrBadRequest, dto.ErrBadRequest, 0, 0},
		{"size above maximum", 1, 101, dto.ErrBadRequest, dto.ErrBadRequest, 0, 0},
		{"minimum integer page", -maxInt - 1, 20, dto.ErrBadRequest, dto.ErrBadRequest, 0, 0},
		{"largest valid offset at size 100", maxInt/100 + 1, 100, nil, nil, maxInt/100 + 1, 100},
		{"first overflowing offset at size 100", maxInt/100 + 2, 100, dto.ErrBadRequest, dto.ErrBadRequest, 0, 0},
		{"maximum page with size one", maxInt, 1, nil, nil, maxInt, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			projects := newProjectReviewRepo()
			projectItems, total, err := service.NewProjectService(projects).List(context.Background(), 100, "PM", model.ProjectFilter{Page: tt.page, PageSize: tt.size})
			if !errors.Is(err, tt.projectErr) {
				t.Fatalf("Project error=%v want=%v", err, tt.projectErr)
			}
			if err == nil {
				if len(projectItems) != 1 || total != 1 || projects.filter.Page != tt.wantPage || projects.filter.PageSize != tt.wantSize {
					t.Fatalf("Project items=%v total=%d filter=%#v", projectItems, total, projects.filter)
				}
			} else if projects.filter.MemberUserID != nil {
				t.Fatal("invalid Project pagination reached the repository")
			}

			advances := &advanceListRepoStub{items: []model.AdvanceListItem{{ID: 7, Amount: "100.00"}}, total: 1}
			advanceItems, total, err := service.NewAdvanceService(advances).List(context.Background(), 100, "PM", model.AdvanceFilter{Page: tt.page, PageSize: tt.size})
			if !errors.Is(err, tt.advanceErr) {
				t.Fatalf("Advance error=%v want=%v", err, tt.advanceErr)
			}
			if err == nil {
				if len(advanceItems) != 1 || total != 1 || advances.filter.Page != tt.wantPage || advances.filter.PageSize != tt.wantSize {
					t.Fatalf("Advance items=%v total=%d filter=%#v", advanceItems, total, advances.filter)
				}
			} else if advances.listCalls != 0 {
				t.Fatal("invalid Advance pagination reached the repository")
			}
		})
	}
}

func TestProjectAndAdvanceHTTPPaginationMatchesMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, resource := range []string{"projects", "advances"} {
		for _, tt := range []struct {
			name, query        string
			status, page, size int
		}{
			{"omitted", "", 200, 1, 20},
			{"both zero", "page=0&page_size=0", 200, 1, 20},
			{"page zero", "page=0&page_size=10", 200, 1, 10},
			{"size zero", "page=3&page_size=0", 200, 3, 20},
			{"explicit page", "page=2&page_size=7", 200, 2, 7},
			{"maximum size", "page_size=100", 200, 1, 100},
			{"negative page", "page=-1", 400, 0, 0},
			{"negative size", "page_size=-1", 400, 0, 0},
			{"oversized page", "page_size=101", 400, 0, 0},
			{"nonnumeric page", "page=abc", 400, 0, 0},
			{"overflowing offset", "page=" + strconv.Itoa(int(^uint(0)>>1)) + "&page_size=100", 400, 0, 0},
		} {
			t.Run(resource+"/"+tt.name, func(t *testing.T) {
				projects := newProjectReviewRepo()
				advances := &advanceListRepoStub{}
				r := gin.New()
				r.Use(func(c *gin.Context) {
					c.Set(middleware.ContextKeyUserID, int64(100))
					c.Set(middleware.ContextKeyRole, "PM")
				})
				path := "/api/v1/" + resource
				if resource == "projects" {
					r.GET(path, handler.NewProjectHandler(service.NewProjectService(projects)).List)
				} else {
					r.GET(path, handler.NewAdvanceHandler(service.NewAdvanceService(advances)).List)
				}
				response := httptest.NewRecorder()
				r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path+"?"+tt.query, nil))
				if response.Code != tt.status {
					t.Fatalf("status=%d want=%d body=%s", response.Code, tt.status, response.Body.String())
				}
				if tt.status != http.StatusOK {
					if projects.filter.MemberUserID != nil || advances.listCalls != 0 {
						t.Fatal("invalid HTTP pagination reached the repository")
					}
					return
				}
				var body struct {
					Success bool     `json:"success"`
					Meta    dto.Meta `json:"meta"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if !body.Success || body.Meta.Page != tt.page || body.Meta.PerPage != tt.size {
					t.Fatalf("pagination metadata=%#v", body)
				}
				page, size := projects.filter.Page, projects.filter.PageSize
				if resource == "advances" {
					page, size = advances.filter.Page, advances.filter.PageSize
				}
				if page != tt.page || size != tt.size {
					t.Fatalf("repository pagination=%d/%d, want=%d/%d", page, size, tt.page, tt.size)
				}
			})
		}
	}
}
