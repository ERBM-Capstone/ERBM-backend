package handler

import (
	"context"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/gin-gonic/gin"
)

type roleService interface {
	List(ctx context.Context) ([]dto.RoleResponse, error)
}

// RoleHandler exposes role-management lookup endpoints.
type RoleHandler struct {
	service roleService
}

// NewRoleHandler creates a RoleHandler backed by svc.
func NewRoleHandler(svc roleService) *RoleHandler {
	return &RoleHandler{service: svc}
}

// List godoc
// @Summary      List application roles
// @Description  Returns roles available for employee provisioning. Requires the ADMIN role.
// @Tags         roles
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  dto.BaseResponse
// @Failure      401  {object}  dto.BaseResponse
// @Failure      403  {object}  dto.BaseResponse
// @Failure      500  {object}  dto.BaseResponse
// @Router       /api/v1/roles [get]
func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.service.List(c.Request.Context())
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, roles)
}
