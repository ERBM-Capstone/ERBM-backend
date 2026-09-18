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
