package handler

import (
	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/gin-gonic/gin"
)

// Disburse godoc
// @Summary      Confirm full disbursement of an approved advance
// @Description  Only ACCOUNTING can confirm a single full payment of the stored approved amount. No body is required; client fields cannot change the amount, actor or payment time. The server records payment and confirmation time, activated_at and a DISBURSE audit event, and changes APPROVED to ACTIVE in one transaction. Returns the existing Advance detail response. A closed Project, non-APPROVED Advance, previous payment or another ACTIVE Advance in the Project returns 409. Concurrent confirmations cannot create duplicate payments or multiple ACTIVE Advances. MD signature remains outside the system in phase 1.
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
// @Router       /api/v1/advances/{id}/disburse [post]
func (h *AdvanceHandler) Disburse(c *gin.Context) {
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
	item, err := h.service.Disburse(c.Request.Context(), userID, role, uri.ID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToAdvanceListItemResponse(*item))
}
