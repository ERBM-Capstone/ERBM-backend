package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/pkg/pagination"
	"github.com/gin-gonic/gin"
)

type projectService interface {
	Create(ctx context.Context, userID int64, role string, req *dto.CreateProjectRequest) (*model.ProjectDetail, error)
	List(ctx context.Context, userID int64, role string, filter model.ProjectFilter) ([]*model.ProjectDetail, int64, error)
	GetByID(ctx context.Context, userID int64, role string, id int64) (*model.ProjectDetail, error)
	Update(ctx context.Context, userID int64, role string, id int64, req *dto.UpdateProjectRequest) (*model.ProjectDetail, error)
}

// ProjectHandler exposes HTTP endpoints for projects.
type ProjectHandler struct {
	service projectService
}

// NewProjectHandler creates a ProjectHandler backed by svc.
func NewProjectHandler(svc projectService) *ProjectHandler {
	return &ProjectHandler{service: svc}
}

// Create godoc
// @Summary      Create project
// @Description  Creates a project with a server-generated project_code: PRJ-{Gregorian year in Asia/Bangkok}-{number, minimum 6 digits}, starting at 000001 each year. Clients do not send project_code; a supplied value is ignored. A project has at most one PC. Optional pc_user_id accepts one positive user ID; the same PC may coordinate multiple projects. ADMIN and PC must send pm_user_id. A PM creator becomes its PM; a PC creator becomes its sole PC and cannot assign another PC at creation.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      dto.CreateProjectRequest  true  "Create project payload"
// @Success      201   {object}  dto.BaseResponse{data=dto.ProjectResponse}
// @Failure      400   {object}  dto.BaseResponse
// @Failure      401   {object}  dto.BaseResponse
// @Failure      403   {object}  dto.BaseResponse
// @Failure      409   {object}  dto.BaseResponse
// @Failure      500   {object}  dto.BaseResponse
// @Router       /api/v1/projects [post]
func (h *ProjectHandler) Create(c *gin.Context) {
	var req dto.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	userID, role, ok := getActor(c)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	project, err := h.service.Create(c.Request.Context(), userID, role, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	dto.Created(c, dto.ToProjectResponse(project))
}

// List godoc
// @Summary      List projects
// @Description  Returns projects in a paginated standard response envelope. ADMIN and ACCOUNTING see every project; PM and PC only see projects they belong to.
// @Tags         projects
// @Produce      json
// @Security     BearerAuth
// @Param        page      query  int     false  "Page number"                  minimum(1)
// @Param        page_size query  int     false  "Items per page"               minimum(1) maximum(100)
// @Param        q         query  string  false  "Search project code or name"
// @Param        status    query  string  false  "Project status"               Enums(ACTIVE, CLOSED)
// @Success      200       {object} dto.BaseResponse{data=[]dto.ProjectResponse}
// @Failure      400       {object} dto.BaseResponse
// @Failure      401       {object} dto.BaseResponse
// @Failure      403       {object} dto.BaseResponse
// @Failure      500       {object} dto.BaseResponse
// @Router       /api/v1/projects [get]
func (h *ProjectHandler) List(c *gin.Context) {
	var query dto.ProjectListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	userID, role, ok := getActor(c)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	page, pageSize := pagination.WithDefaults(query.Page, query.PageSize)

	projects, total, err := h.service.List(c.Request.Context(), userID, role, model.ProjectFilter{
		Query:    query.Q,
		Status:   query.Status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	res := make([]dto.ProjectResponse, 0, len(projects))
	for _, project := range projects {
		res = append(res, dto.ToProjectResponse(project))
	}

	dto.WithMeta(c, http.StatusOK, res, "Projects retrieved", &dto.Meta{
		Page:    page,
		PerPage: pageSize,
		Total:   int(total),
	})
}

// GetByID godoc
// @Summary      Get project
// @Description  Returns a single project with its PM and an assigned pc object or null. PM and PC can only open projects they belong to.
// @Tags         projects
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Project ID"  minimum(1)
// @Success      200  {object}  dto.BaseResponse{data=dto.ProjectResponse}
// @Failure      400  {object}  dto.BaseResponse
// @Failure      401  {object}  dto.BaseResponse
// @Failure      403  {object}  dto.BaseResponse
// @Failure      404  {object}  dto.BaseResponse
// @Failure      500  {object}  dto.BaseResponse
// @Router       /api/v1/projects/{id} [get]
func (h *ProjectHandler) GetByID(c *gin.Context) {
	var uri dto.URIParam
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	userID, role, ok := getActor(c)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	project, err := h.service.GetByID(c.Request.Context(), userID, role, uri.ID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	dto.OK(c, dto.ToProjectResponse(project))
}

// Update godoc
// @Summary      Update project
// @Description  Partially updates a project. Omit pc_user_id to keep its PC, send a positive user ID to replace it, or null to remove it. ADMIN and the project's PM can edit fields and assignments; the assigned PC can only change name and description. Only the project's PM can close it by sending status "CLOSED". Closing returns 409 if any Advance in this Project is not CLOSED, including DRAFT. A closed project cannot be changed.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                       true  "Project ID"  minimum(1)
// @Param        body  body      dto.UpdateProjectRequest  true  "Project update payload"
// @Success      200   {object}  dto.BaseResponse{data=dto.ProjectResponse}
// @Failure      400   {object}  dto.BaseResponse
// @Failure      401   {object}  dto.BaseResponse
// @Failure      403   {object}  dto.BaseResponse
// @Failure      404   {object}  dto.BaseResponse
// @Failure      409   {object}  dto.BaseResponse
// @Failure      500   {object}  dto.BaseResponse
// @Router       /api/v1/projects/{id} [patch]
func (h *ProjectHandler) Update(c *gin.Context) {
	var uri dto.URIParam
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	var req dto.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	userID, role, ok := getActor(c)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	project, err := h.service.Update(c.Request.Context(), userID, role, uri.ID, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	dto.OK(c, dto.ToProjectResponse(project))
}

// getActor reads the authenticated user ID and role set by RequireAuth.
func getActor(c *gin.Context) (int64, string, bool) {
	fmt.Println("getActor called") // Debugging line to check if the function is being called
	userValue, exists := c.Get(middleware.ContextKeyUserID)
	userID, ok := userValue.(int64)
	if !exists || !ok || userID <= 0 {
		return 0, "", false
	}

	roleValue, exists := c.Get(middleware.ContextKeyRole)
	role, ok := roleValue.(string)
	if !exists || !ok || role == "" {
		return 0, "", false
	}

	return userID, role, true
}
