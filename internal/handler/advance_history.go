package handler

import (
	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/gin-gonic/gin"
)

// History godoc
// @Summary      List advance workflow history
// @Description  Returns all recorded workflow actions, newest first by occurred_at DESC and event ID DESC, without pagination. Includes actor ID/current name, previous/resulting status, nullable reason and event timestamp. SUBMIT events include a per-Advance revision number starting at 1 in event-ID order, plus the exact THB amount and purpose from that submission's immutable snapshot. Revision, amount and purpose are null for other actions; missing snapshot fields stay null. No full before/after snapshots are exposed. Uses the same current Project access and private-DRAFT visibility as GET detail. An existing Advance with no events returns an empty array. Reading history does not change the Advance. Replaces the separate revisions endpoint.
// @Tags         advances
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Advance ID" minimum(1)
// @Success      200 {object} dto.BaseResponse{data=[]dto.AdvanceHistoryResponse}
// @Failure      400 {object} dto.BaseResponse
// @Failure      401 {object} dto.BaseResponse
// @Failure      403 {object} dto.BaseResponse
// @Failure      404 {object} dto.BaseResponse
// @Failure      500 {object} dto.BaseResponse
// @Router       /api/v1/advances/{id}/history [get]
func (h *AdvanceHandler) History(c *gin.Context) {
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
	items, err := h.service.History(c.Request.Context(), userID, role, uri.ID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	response := make([]dto.AdvanceHistoryResponse, 0, len(items))
	for _, item := range items {
		response = append(response, dto.ToAdvanceHistoryResponse(item))
	}
	dto.OK(c, response)
}
