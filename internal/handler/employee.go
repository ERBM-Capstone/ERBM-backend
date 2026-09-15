package handler

import (
	"context"
	"net/http"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/gin-gonic/gin"
)

type employeeService interface {
	Create(ctx context.Context, req []*model.Employee) ([]*model.Employee, error)
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
// @Description  Creates employee profiles and invited login accounts for active employees. Role is derived from position. Requires the ADMIN role.
// @Tags         employees
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      []dto.CreateEmployeeRequest  true  "Create employee payloads"
// @Success      201   {array}   dto.EmployeeResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      401   {object}  dto.ErrorResponse
// @Failure      403   {object}  dto.ErrorResponse
// @Failure      409   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /api/v1/employees [post]
func (h *EmployeeHandler) Create(c *gin.Context) {
	var req []dto.CreateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	var employees []*model.Employee
	for _, r := range req {
		employee, err := dto.ToEmployee(&r)
		if err != nil {
			dto.HandleError(c, dto.ErrBadRequest)
			return
		}
		employees = append(employees, employee)
	}

	employees, err := h.service.Create(c.Request.Context(), employees)
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	res := make([]dto.EmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		res = append(res, dto.ToEmployeeResponse(employee))
	}

	c.JSON(http.StatusCreated, res)
}
