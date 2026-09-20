package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type employeeService interface {
	Create(ctx context.Context, actorID int64, req []service.EmployeeProvision) ([]*model.Employee, error)
	List(ctx context.Context, page int, pageSize int) ([]*model.Employee, int64, error)
}

// EmployeeHandler exposes HTTP endpoints for employees.
type EmployeeHandler struct {
	service employeeService
}

// NewEmployeeHandler creates an EmployeeHandler backed by svc.
func NewEmployeeHandler(svc employeeService) *EmployeeHandler {
	return &EmployeeHandler{service: svc}
}

// Create godoc
// @Summary      Create employee
// @Description  Creates employee profiles and invited login accounts for active employees. role_code is required and independent of the HR position. Requires the ADMIN role.
// @Tags         employees
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      []dto.CreateEmployeeRequest  true  "Create employee payloads"
// @Success      201   {object}  dto.BaseResponse
// @Failure      400   {object}  dto.BaseResponse
// @Failure      401   {object}  dto.BaseResponse
// @Failure      403   {object}  dto.BaseResponse
// @Failure      409   {object}  dto.BaseResponse
// @Failure      500   {object}  dto.BaseResponse
// @Router       /api/v1/employees [post]
func (h *EmployeeHandler) Create(c *gin.Context) {
	var req []dto.CreateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	actorValue, exists := c.Get(middleware.ContextKeyUserID)
	actorID, ok := actorValue.(int64)
	if !exists || !ok || actorID <= 0 {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	provisions := make([]service.EmployeeProvision, 0, len(req))
	seenEmails := make(map[string]struct{}, len(req))
	for _, r := range req {
		employee, err := dto.ToEmployee(&r)
		if err != nil {
			dto.HandleError(c, dto.ErrBadRequest)
			return
		}
		if _, exists := seenEmails[employee.Email]; exists {
			dto.HandleError(c, dto.ErrEmployeeEmailExists)
			return
		}
		seenEmails[employee.Email] = struct{}{}

		provisions = append(provisions, service.EmployeeProvision{
			Employee: employee,
			RoleCode: strings.ToUpper(strings.TrimSpace(r.RoleCode)),
		})
	}

	employees, err := h.service.Create(c.Request.Context(), actorID, provisions)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	res := make([]dto.EmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		res = append(res, dto.ToEmployeeResponse(employee))
	}

	dto.Created(c, res)
}

// List godoc
// @Summary      List employees
// @Description  Returns employees in a paginated standard response envelope. Requires the ADMIN role.
// @Tags         employees
// @Produce      json
// @Security     BearerAuth
// @Param        page      query  int  false  "Page number"       minimum(1)
// @Param        page_size query  int  false  "Items per page"    minimum(1) maximum(100)
// @Success      200       {object} dto.BaseResponse
// @Failure      400       {object} dto.BaseResponse
// @Failure      401       {object} dto.BaseResponse
// @Failure      403       {object} dto.BaseResponse
// @Failure      500       {object} dto.BaseResponse
// @Router       /api/v1/employees [get]
func (h *EmployeeHandler) List(c *gin.Context) {
	var query dto.ListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	employees, total, err := h.service.List(c.Request.Context(), query.Page, query.PageSize)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	res := make([]dto.EmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		res = append(res, dto.ToEmployeeResponse(employee))
	}

	dto.WithMeta(c, http.StatusOK, res, "Employees retrieved", &dto.Meta{
		Page:    query.Page,
		PerPage: query.PageSize,
		Total:   int(total),
	})
}
