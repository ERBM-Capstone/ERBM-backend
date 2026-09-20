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
	GetByID(ctx context.Context, ID int64) (*model.Employee, error)
	Update(ctx context.Context, employeeID int64, req *dto.UpdateEmployeeRequest, actorID int64) (*model.Employee, error)
	Deactivate(ctx context.Context, employeeID int64, actorID int64, endDate string) error
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
	page := query.Page
	if page == 0 {
		page = 1
	}

	pageSize := query.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	employees, total, err := h.service.List(c.Request.Context(), page, pageSize)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	res := make([]dto.EmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		res = append(res, dto.ToEmployeeResponse(employee))
	}

	dto.WithMeta(c, http.StatusOK, res, "Employees retrieved", &dto.Meta{
		Page:    page,
		PerPage: pageSize,
		Total:   int(total),
	})
}

// GetByID godoc
// @Summary      Get employee
// @Description  Returns a single employee by ID. Requires the ADMIN role.
// @Tags         employees
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Employee ID"  minimum(1)
// @Success      200  {object}  dto.BaseResponse
// @Failure      400  {object}  dto.BaseResponse
// @Failure      401  {object}  dto.BaseResponse
// @Failure      403  {object}  dto.BaseResponse
// @Failure      404  {object}  dto.BaseResponse
// @Failure      500  {object}  dto.BaseResponse
// @Router       /api/v1/employees/{id} [get]
func (h *EmployeeHandler) GetByID(c *gin.Context) {
	var uri dto.URIParam
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	employee, err := h.service.GetByID(c.Request.Context(), uri.ID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToEmployeeResponse(employee))

}

// Update godoc
// @Summary      Update employee
// @Description  Partially updates an employee profile. Requires the ADMIN role.
// @Tags         employees
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                        true  "Employee ID"  minimum(1)
// @Param        body  body      dto.UpdateEmployeeRequest  true  "Employee update payload"
// @Success      200   {object}  dto.BaseResponse
// @Failure      400   {object}  dto.BaseResponse
// @Failure      401   {object}  dto.BaseResponse
// @Failure      403   {object}  dto.BaseResponse
// @Failure      404   {object}  dto.BaseResponse
// @Failure      500   {object}  dto.BaseResponse
// @Router       /api/v1/employees/{id} [patch]
func (h *EmployeeHandler) Update(c *gin.Context) {
	var uri dto.URIParam
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	actorValue, exists := c.Get(middleware.ContextKeyUserID)
	actorID, ok := actorValue.(int64)
	if !exists || !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	var req dto.UpdateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	updatedEmployee, err := h.service.Update(c.Request.Context(), uri.ID, &req, actorID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	dto.OK(c, dto.ToEmployeeResponse(updatedEmployee))
}

// Deactivate godoc
// @Summary      Deactivate employee
// @Description  Records an employee's employment end date and deactivates the associated login account. Requires the ADMIN role.
// @Tags         employees
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                             true  "Employee ID"  minimum(1)
// @Param        body  body      dto.DeactivateEmployeeRequest  true  "Employment end date"
// @Success      200   {object}  dto.BaseResponse
// @Failure      400   {object}  dto.BaseResponse
// @Failure      401   {object}  dto.BaseResponse
// @Failure      403   {object}  dto.BaseResponse
// @Failure      404   {object}  dto.BaseResponse
// @Failure      500   {object}  dto.BaseResponse
// @Router       /api/v1/employees/{id}/deactivate [patch]
func (h *EmployeeHandler) Deactivate(c *gin.Context) {
	var uri dto.URIParam
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	var req dto.DeactivateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	actorValue, exists := c.Get(middleware.ContextKeyUserID)
	actorID, ok := actorValue.(int64)
	if !exists || !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	err := h.service.Deactivate(c.Request.Context(), actorID, uri.ID, req.EmploymentEndDate)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	dto.OK(c, gin.H{"message": "Employee deactivated successfully"})
}
