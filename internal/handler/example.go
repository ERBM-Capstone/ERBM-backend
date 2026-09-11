package handler

import (
	"net/http"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ExampleHandler exposes HTTP endpoints for the Example resource.
type ExampleHandler struct {
	service *service.ExampleService
}

// NewExampleHandler creates a new ExampleHandler backed by svc.
func NewExampleHandler(svc *service.ExampleService) *ExampleHandler {
	return &ExampleHandler{service: svc}
}

// List godoc
// @Summary      List examples
// @Description  Returns all non-deleted examples ordered by updated_at DESC.
// @Tags         examples
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   dto.ExampleResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/examples [get]
func (h *ExampleHandler) List(c *gin.Context) {
	examples, err := h.service.List(c.Request.Context())
	if err != nil {
		dto.HandleError(c, err)
		return
	}

	resp := make([]dto.ExampleResponse, 0, len(examples))
	for i := range examples {
		resp = append(resp, dto.ToExampleResponse(&examples[i]))
	}
	c.JSON(http.StatusOK, resp)
}

// GetByID godoc
// @Summary      Get example by ID
// @Description  Returns a single non-deleted example.
// @Tags         examples
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Example UUID"
// @Success      200  {object}  dto.ExampleResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Router       /api/v1/examples/{id} [get]
func (h *ExampleHandler) GetByID(c *gin.Context) {
	var uri dto.ExampleID
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	example, err := h.service.GetByID(c.Request.Context(), uri.ID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ToExampleResponse(example))
}

// Create godoc
// @Summary      Create example
// @Description  Inserts a new example. Requires admin or editor role.
// @Tags         examples
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      dto.CreateExampleRequest  true  "Create payload"
// @Success      201   {object}  dto.ExampleResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      401   {object}  dto.ErrorResponse
// @Router       /api/v1/examples [post]
func (h *ExampleHandler) Create(c *gin.Context) {
	userID, ok := c.Get(middleware.ContextKeyUserID)
	if !ok || userID == nil {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	uid, ok := userID.(string)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	var req dto.CreateExampleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	example, err := h.service.Create(c.Request.Context(), &req, uid)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.ToExampleResponse(example))
}

// Update godoc
// @Summary      Update example
// @Description  Partially updates an existing non-deleted example. Requires admin or editor role.
// @Tags         examples
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string                    true  "Example UUID"
// @Param        body  body      dto.UpdateExampleRequest  true  "Update payload"
// @Success      200   {object}  dto.ExampleResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      401   {object}  dto.ErrorResponse
// @Failure      404   {object}  dto.ErrorResponse
// @Router       /api/v1/examples/{id} [put]
func (h *ExampleHandler) Update(c *gin.Context) {
	userID, ok := c.Get(middleware.ContextKeyUserID)
	if !ok || userID == nil {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	uid, ok := userID.(string)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	var uri dto.ExampleID
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	var req dto.UpdateExampleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	example, err := h.service.Update(c.Request.Context(), uri.ID, &req, uid)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ToExampleResponse(example))
}

// Delete godoc
// @Summary      Delete example
// @Description  Soft-deletes an example. Requires admin or editor role.
// @Tags         examples
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Example UUID"
// @Success      200  {object}  dto.DeleteResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      401  {object}  dto.ErrorResponse
// @Failure      410  {object}  dto.ErrorResponse
// @Router       /api/v1/examples/{id} [delete]
func (h *ExampleHandler) Delete(c *gin.Context) {
	userID, ok := c.Get(middleware.ContextKeyUserID)
	if !ok || userID == nil {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	uid, ok := userID.(string)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return
	}

	var uri dto.ExampleID
	if err := c.ShouldBindUri(&uri); err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}

	if err := h.service.Delete(c.Request.Context(), uri.ID, uid); err != nil {
		dto.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.DeleteResponse{Message: "example deleted successfully"})
}
