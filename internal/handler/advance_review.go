package handler

import (
	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/gin-gonic/gin"
)

// Approve godoc
// @Summary      Approve an advance request
// @Description  Only the Project's current PM can approve a PENDING_PM_APPROVAL request. No body is required. Sets APPROVED, approved_at/by and updated_at/by; preserves the submitted contents and writes an APPROVE audit event atomically. Repeated or conflicting review returns 409. A closed Project cannot be changed.
// @Tags         advances
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Advance ID" minimum(1)
// @Success      200 {object} dto.BaseResponse{data=dto.AdvanceListItemResponse}
// @Failure      400 {object} dto.BaseResponse
// @Failure      401 {object} dto.BaseResponse
// @Failure      403 {object} dto.BaseResponse
// @Failure      404 {object} dto.BaseResponse
// @Failure      409 {object} dto.BaseResponse
// @Failure      500 {object} dto.BaseResponse
// @Router       /api/v1/advances/{id}/approve [post]
func (h *AdvanceHandler) Approve(c *gin.Context) {
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
	item, err := h.service.Approve(c.Request.Context(), userID, role, uri.ID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToAdvanceListItemResponse(*item))
}

// Return godoc
// @Summary      Return an advance request for revision
// @Description  Only the Project's current PM can return a PENDING_PM_APPROVAL request. A nonblank reason is required and trimmed before storage in the RETURN audit event. Sets RETURNED_FOR_REVISION and updated_at/by while preserving request contents and submission metadata. The change and audit commit together. Other statuses or a closed Project return 409.
// @Tags         advances
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Advance ID" minimum(1)
// @Param        body body dto.ReturnAdvanceRequest true "Reason for returning the request"
// @Success      200 {object} dto.BaseResponse{data=dto.AdvanceListItemResponse}
// @Failure      400 {object} dto.BaseResponse
// @Failure      401 {object} dto.BaseResponse
// @Failure      403 {object} dto.BaseResponse
// @Failure      404 {object} dto.BaseResponse
// @Failure      409 {object} dto.BaseResponse
// @Failure      500 {object} dto.BaseResponse
// @Router       /api/v1/advances/{id}/return [post]
func (h *AdvanceHandler) Return(c *gin.Context) {
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
	var req dto.ReturnAdvanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	item, err := h.service.Return(c.Request.Context(), userID, role, uri.ID, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToAdvanceListItemResponse(*item))
}
