package handler

import (
	"context"
	"net/http"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/pkg/pagination"
	"github.com/gin-gonic/gin"
)

type advanceService interface {
	GetByID(context.Context, int64, string, int64) (*model.AdvanceListItem, error)
	Update(context.Context, int64, string, int64, *dto.UpdateAdvanceRequest) (*model.AdvanceListItem, error)
	List(context.Context, int64, string, model.AdvanceFilter) ([]model.AdvanceListItem, int64, error)
	Create(context.Context, int64, string, *dto.CreateAdvanceRequest) (*model.AdvanceListItem, error)
}

// GetByID godoc
// @Summary      Get an advance
// @Description  Returns the current Advance with the same fields and visibility as the list. ADMIN and ACCOUNTING can read every Project; PM and PC must be assigned to its Project. DRAFT is private to its creator for every role.
// @Tags         advances
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Advance ID" minimum(1)
// @Success      200 {object} dto.BaseResponse{data=dto.AdvanceListItemResponse}
// @Failure      400 {object} dto.BaseResponse
// @Failure      401 {object} dto.BaseResponse
// @Failure      403 {object} dto.BaseResponse
// @Failure      404 {object} dto.BaseResponse
// @Failure      500 {object} dto.BaseResponse
// @Router       /api/v1/advances/{id} [get]
func (h *AdvanceHandler) GetByID(c *gin.Context) {
	userID, role, ok := getActor(c)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}
	var uri dto.URIParam
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	item, err := h.service.GetByID(c.Request.Context(), userID, role, uri.ID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToAdvanceListItemResponse(*item))
}

// Update godoc
// @Summary      Update an advance draft
// @Description  Only the PC creator who is still assigned to the Project can edit purpose and/or amount. Omitted or null fields retain their values; at least one non-null field is required. Amount uses the positive decimal-string THB rules of creation. Only DRAFT or RETURNED_FOR_REVISION can be edited; the result is DRAFT. A closed Project cannot be changed. Update and before/after audit snapshots commit together.
// @Tags         advances
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Advance ID" minimum(1)
// @Param        body body dto.UpdateAdvanceRequest true "Advance update payload"
// @Success      200 {object} dto.BaseResponse{data=dto.AdvanceListItemResponse}
// @Failure      400 {object} dto.BaseResponse
// @Failure      401 {object} dto.BaseResponse
// @Failure      403 {object} dto.BaseResponse
// @Failure      404 {object} dto.BaseResponse
// @Failure      409 {object} dto.BaseResponse
// @Failure      500 {object} dto.BaseResponse
// @Router       /api/v1/advances/{id} [patch]
func (h *AdvanceHandler) Update(c *gin.Context) {
	userID, role, ok := getActor(c)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}
	var uri dto.URIParam
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	var req dto.UpdateAdvanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	item, err := h.service.Update(c.Request.Context(), userID, role, uri.ID, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToAdvanceListItemResponse(*item))
}

type AdvanceHandler struct{ service advanceService }

func NewAdvanceHandler(svc advanceService) *AdvanceHandler { return &AdvanceHandler{service: svc} }

// Create godoc
// @Summary      Create an advance draft
// @Description  PC assigned to an ACTIVE Project creates a DRAFT with a required positive THB amount. Codes use ADV-{project_code}-{per-project sequence}, assigned once at creation. An existing ACTIVE Advance does not block DRAFT creation. Creation, code allocation and audit event commit together. Requester is the authenticated creator.
// @Tags         advances
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body dto.CreateAdvanceRequest true "Create Advance draft"
// @Success      201 {object} dto.BaseResponse{data=dto.AdvanceListItemResponse}
// @Failure      400 {object} dto.BaseResponse
// @Failure      401 {object} dto.BaseResponse
// @Failure      403 {object} dto.BaseResponse
// @Failure      404 {object} dto.BaseResponse
// @Failure      409 {object} dto.BaseResponse
// @Failure      500 {object} dto.BaseResponse
// @Router       /api/v1/advances [post]
func (h *AdvanceHandler) Create(c *gin.Context) {
	userID, role, ok := getActor(c)

	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}
	var req dto.CreateAdvanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	item, err := h.service.Create(c.Request.Context(), userID, role, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.Created(c, dto.ToAdvanceListItemResponse(*item))
}

// List godoc
// @Summary      List advances
// @Description  Lists visible Advances in THB. ADMIN and ACCOUNTING can read every Project; PM sees managed Projects and PC sees assigned Projects. DRAFT is visible only to its creator for every role. Visibility applies before pagination and total count. Sorted by created_at DESC, id DESC.
// @Tags         advances
// @Produce      json
// @Security     BearerAuth
// @Param        project_id query int    false "Filter by accessible Project ID" minimum(1)
// @Param        status     query string false "Advance status" Enums(DRAFT, PENDING_PM_APPROVAL, RETURNED_FOR_REVISION, APPROVED, ACTIVE, CLOSED)
// @Param        q          query string false "Case-insensitive search in purpose" maxlength(255)
// @Param        page       query int    false "Page number" default(1) minimum(1)
// @Param        page_size  query int    false "Items per page" default(20) minimum(1) maximum(100)
// @Success      200 {object} dto.BaseResponse{data=[]dto.AdvanceListItemResponse}
// @Failure      400 {object} dto.BaseResponse
// @Failure      401 {object} dto.BaseResponse
// @Failure      403 {object} dto.BaseResponse
// @Failure      404 {object} dto.BaseResponse
// @Failure      500 {object} dto.BaseResponse
// @Router       /api/v1/advances [get]
func (h *AdvanceHandler) List(c *gin.Context) {
	userID, role, ok := getActor(c)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}
	var query dto.AdvanceListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	page, pageSize := pagination.WithDefaults(query.Page, query.PageSize)

	items, total, err := h.service.List(c.Request.Context(), userID, role, model.AdvanceFilter{
		ProjectID: query.ProjectID, Status: query.Status, Query: query.Q, Page: page, PageSize: pageSize,
	})
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	response := make([]dto.AdvanceListItemResponse, 0, len(items))
	for _, item := range items {
		response = append(response, dto.ToAdvanceListItemResponse(item))
	}
	dto.WithMeta(c, http.StatusOK, response, "Advances retrieved", &dto.Meta{
		Page:    page,
		PerPage: pageSize,
		Total:   int(total),
	})
}
