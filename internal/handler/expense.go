package handler

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/storage"
	"github.com/gin-gonic/gin"
)

type expenseService interface {
	Create(context.Context, int64, string, int64, *dto.CreateExpenseRequest) (*model.Expense, error)
	Get(context.Context, int64, string, int64) (*model.Expense, error)
	List(context.Context, int64, string, int64, int, int) ([]model.Expense, int64, error)
	Update(context.Context, int64, string, int64, *dto.UpdateExpenseRequest) (*model.Expense, error)
	Confirm(context.Context, int64, string, int64, *dto.ConfirmExpenseRequest) (*model.Expense, error)
	Reopen(context.Context, int64, string, int64, *dto.ReopenExpenseRequest) (*model.Expense, error)
	Delete(context.Context, int64, string, int64) (bool, error)
	Reconciliation(context.Context, int64, string, int64) (*dto.ReconciliationResponse, error)
	UploadReceipt(context.Context, int64, string, int64, string, []byte) (*model.ExpenseDocument, error)
	Receipts(context.Context, int64, string, int64) ([]model.ExpenseDocument, error)
	OpenReceipt(context.Context, int64, string, int64, int64) (*model.ExpenseDocument, io.ReadCloser, error)
	DeleteReceipt(context.Context, int64, string, int64, int64) (bool, error)
	ReadOCR(context.Context, int64, string, int64, *dto.ReadReceiptRequest) (*dto.OCRResponse, error)
	OCRResult(context.Context, int64, string, int64, dto.OCRResultQuery) (*dto.OCRResponse, error)
}
type ExpenseHandler struct{ service expenseService }

func NewExpenseHandler(s expenseService) *ExpenseHandler { return &ExpenseHandler{service: s} }
func expenseIdentity(c *gin.Context) (int64, string, int64, bool) {
	actor, role, ok := getActor(c)
	if !ok {
		dto.HandleError(c, dto.ErrUnauthorized)
		return 0, "", 0, false
	}
	var uri dto.URIParam
	if c.ShouldBindUri(&uri) != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return 0, "", 0, false
	}
	return actor, role, uri.ID, true
}
func expenseJSON(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2_000_000)
	if c.ShouldBindJSON(v) != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return false
	}
	return true
}

// Create godoc
// @Summary Create an unconfirmed Expense before uploading receipts
// @Description Only the assigned PC creator of an ACTIVE Advance. The empty object is valid; amount starts null and status starts DRAFT.
// @Tags expenses
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Advance ID"
// @Param body body dto.CreateExpenseRequest true "Draft"
// @Success 201 {object} dto.BaseResponse{data=dto.ExpenseResponse}
// @Failure 400,401,403,404,409,500 {object} dto.BaseResponse
// @Router /api/v1/advances/{id}/expenses [post]
func (h *ExpenseHandler) Create(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var req dto.CreateExpenseRequest
	if !expenseJSON(c, &req) {
		return
	}
	v, err := h.service.Create(c.Request.Context(), a, r, id, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.Created(c, dto.ToExpenseResponse(*v))
}

// List godoc
// @Summary List Advance expenses
// @Tags expenses
// @Produce json
// @Security BearerAuth
// @Param id path int true "Advance ID"
// @Param page query int false "Page" default(1)
// @Param page_size query int false "Page size" default(20) maximum(100)
// @Success 200 {object} dto.BaseResponse{data=[]dto.ExpenseResponse}
// @Failure 400,401,403,404,500 {object} dto.BaseResponse
// @Router /api/v1/advances/{id}/expenses [get]
func (h *ExpenseHandler) List(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var q dto.ListQuery
	if c.ShouldBindQuery(&q) != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	rows, n, err := h.service.List(c.Request.Context(), a, r, id, q.Page, q.PageSize)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	data := make([]dto.ExpenseResponse, 0, len(rows))
	for _, v := range rows {
		data = append(data, dto.ToExpenseResponse(v))
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	dto.WithMeta(c, http.StatusOK, data, "OK", &dto.Meta{Page: q.Page, PerPage: q.PageSize, Total: int(n)})
}

// Get godoc
// @Summary Get an Expense
// @Tags expenses
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Success 200 {object} dto.BaseResponse{data=dto.ExpenseResponse}
// @Failure 400,401,403,404,500 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id} [get]
func (h *ExpenseHandler) Get(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	v, err := h.service.Get(c.Request.Context(), a, r, id)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToExpenseResponse(*v))
}

// Update godoc
// @Summary Correct the editable Expense draft
// @Description expected_version prevents overwriting newer edits. Only DRAFT expenses can be patched. Returns 409 while OCR is processing any receipt in the Expense. Omitted fields are retained; null clears nullable fields. A single amount stores the reviewed price as a decimal string, zero allowed. Only CONFIRMED expenses count toward money totals.
// @Tags expenses
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Param body body dto.UpdateExpenseRequest true "Draft changes"
// @Success 200 {object} dto.BaseResponse{data=dto.ExpenseResponse}
// @Failure 400,401,403,404,409,500 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id} [patch]
func (h *ExpenseHandler) Update(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var req dto.UpdateExpenseRequest
	if !expenseJSON(c, &req) {
		return
	}
	v, err := h.service.Update(c.Request.Context(), a, r, id, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToExpenseResponse(*v))
}

// Confirm godoc
// @Summary Confirm the reviewed Expense draft
// @Description Marks the reviewed amount CONFIRMED without copying it to another amount field. Returns 409 while OCR is processing any receipt in the Expense. Nonnegative THB only; total confirmed expenses cannot exceed the Advance. Confirming the same version again is idempotent.
// @Tags expenses
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Param body body dto.ConfirmExpenseRequest true "Version reviewed by PC"
// @Success 200 {object} dto.BaseResponse{data=dto.ExpenseResponse}
// @Failure 400,401,403,404,409,500 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id}/confirm [post]
func (h *ExpenseHandler) Confirm(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var req dto.ConfirmExpenseRequest
	if !expenseJSON(c, &req) {
		return
	}
	v, err := h.service.Confirm(c.Request.Context(), a, r, id, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToExpenseResponse(*v))
}

// Reopen godoc
// @Summary Withdraw confirmation and return an Expense to review
// @Description Only the assigned PC creator while the Advance is ACTIVE and no OCR is processing in the Expense. Keeps the single amount, clears confirmation, and removes the Expense from actual totals until PC confirms again. Increments version to invalidate stale forms. The change and audit commit atomically.
// @Tags expenses
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Param body body dto.ReopenExpenseRequest true "Current version"
// @Success 200 {object} dto.BaseResponse{data=dto.ExpenseResponse}
// @Failure 400,401,403,404,409,500 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id}/reopen [post]
func (h *ExpenseHandler) Reopen(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var req dto.ReopenExpenseRequest
	if !expenseJSON(c, &req) {
		return
	}
	v, err := h.service.Reopen(c.Request.Context(), a, r, id, &req)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, dto.ToExpenseResponse(*v))
}

// Delete godoc
// @Summary Permanently delete an Expense and its receipts
// @Description ACTIVE only. Database rows are hard deleted. Physical deletion is retried durably when storage is unavailable; cleanup_pending reports this condition.
// @Tags expenses
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Success 204 "Deleted, including receipt bytes"
// @Success 202 {object} dto.BaseResponse{data=dto.DeletionResponse}
// @Failure 400,401,403,404,409,500 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id} [delete]
func (h *ExpenseHandler) Delete(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	pending, err := h.service.Delete(c.Request.Context(), a, r, id)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	if pending {
		dto.Success(c, http.StatusAccepted, dto.DeletionResponse{CleanupPending: true}, "Receipt cleanup pending")
	} else {
		dto.NoContent(c)
	}
}

// Reconciliation godoc
// @Summary Calculate Advance reconciliation using confirmed amounts
// @Tags expenses
// @Produce json
// @Security BearerAuth
// @Param id path int true "Advance ID"
// @Success 200 {object} dto.BaseResponse{data=dto.ReconciliationResponse}
// @Failure 400,401,403,404,500 {object} dto.BaseResponse
// @Router /api/v1/advances/{id}/reconciliation [get]
func (h *ExpenseHandler) Reconciliation(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	v, err := h.service.Reconciliation(c.Request.Context(), a, r, id)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, v)
}

// UploadReceipt godoc
// @Summary Upload one private receipt
// @Description JPG/JPEG, PNG or PDF, validated from actual bytes. Maximum 10 MB/file, five files/Expense, 50 MB total. PDF max five pages, unencrypted.
// @Tags receipts
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Param file formData file true "Receipt"
// @Success 201 {object} dto.BaseResponse{data=dto.ReceiptResponse}
// @Failure 400,401,403,404,409,413,415,422,503 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id}/receipt [post]
// @Router /api/v1/expenses/{id}/receipts [post]
func (h *ExpenseHandler) UploadReceipt(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, storage.MaxReceiptBytes+65536)
	err := c.Request.ParseMultipartForm(storage.MaxReceiptBytes + 65536)
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			dto.HandleError(c, dto.ErrFileTooLarge)
		} else {
			dto.HandleError(c, dto.ErrBadRequest)
		}
		return
	}
	form := c.Request.MultipartForm
	if form == nil || len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value) != 0 {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	f := form.File["file"][0]
	if f.Size > storage.MaxReceiptBytes {
		dto.HandleError(c, dto.ErrFileTooLarge)
		return
	}
	in, err := f.Open()
	if err != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	defer in.Close()
	b, err := storage.ReadBounded(in)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	v, err := h.service.UploadReceipt(c.Request.Context(), a, r, id, f.Filename, b)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.Created(c, dto.ToReceiptResponse(*v))
}

// Receipts godoc
// @Summary List private receipt metadata
// @Tags receipts
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Success 200 {object} dto.BaseResponse{data=[]dto.ReceiptResponse}
// @Failure 400,401,403,404,500 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id}/receipts [get]
func (h *ExpenseHandler) Receipts(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	rows, err := h.service.Receipts(c.Request.Context(), a, r, id)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	data := make([]dto.ReceiptResponse, 0, len(rows))
	for _, v := range rows {
		data = append(data, dto.ToReceiptResponse(v))
	}
	dto.OK(c, data)
}

// OpenReceipt godoc
// @Summary Open a receipt after checking Project access
// @Tags receipts
// @Produce application/octet-stream
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Param receipt_id path int true "Receipt ID"
// @Success 200 {file} binary
// @Failure 400,401,403,404,503 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id}/receipts/{receipt_id}/file [get]
func (h *ExpenseHandler) OpenReceipt(c *gin.Context) {
	a, r, _, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var uri dto.ReceiptURI
	if c.ShouldBindUri(&uri) != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	d, in, err := h.service.OpenReceipt(c.Request.Context(), a, r, uri.ID, uri.ReceiptID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	defer in.Close()
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": d.OriginalFilename}))
	c.DataFromReader(http.StatusOK, d.SizeBytes, d.ContentType, in, nil)
}

// DeleteReceipt godoc
// @Summary Permanently delete an incorrect receipt and its OCR results
// @Description Does not change the PC-confirmed amount or draft. ACTIVE only.
// @Tags receipts
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Param receipt_id path int true "Receipt ID"
// @Success 204 "Deleted, including receipt bytes"
// @Success 202 {object} dto.BaseResponse{data=dto.DeletionResponse}
// @Failure 400,401,403,404,409,500 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id}/receipts/{receipt_id} [delete]
func (h *ExpenseHandler) DeleteReceipt(c *gin.Context) {
	a, r, _, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var uri dto.ReceiptURI
	if c.ShouldBindUri(&uri) != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	pending, err := h.service.DeleteReceipt(c.Request.Context(), a, r, uri.ID, uri.ReceiptID)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	if pending {
		dto.Success(c, http.StatusAccepted, dto.DeletionResponse{CleanupPending: true}, "Receipt cleanup pending")
	} else {
		dto.NoContent(c)
	}
}

// ReadOCR godoc
// @Summary Read a single receipt and fill the editable draft
// @Description Synchronous 30-second deadline for the whole file. One OCR at a time per Expense across all receipts; PATCH, confirm, reopen and another OCR request return 409 while processing. Success autofills the DRAFT for PC review; OCR never confirms. Failure, cancellation and timeout release the lock and allow retry; failed attempts are not reused as cache. Successful cache is reused unless force=true. The custom private provider adapter must be configured. Failures include durable extraction metadata in errors.
// @Tags ocr
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Param body body dto.ReadReceiptRequest true "Receipt and rerun flag"
// @Success 200 {object} dto.BaseResponse{data=dto.OCRResponse}
// @Failure 400,401,403,404,409,502,503,504 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id}/ocr [post]
func (h *ExpenseHandler) ReadOCR(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var req dto.ReadReceiptRequest
	if !expenseJSON(c, &req) {
		return
	}
	v, err := h.service.ReadOCR(c.Request.Context(), a, r, id, &req)
	if err != nil {
		if v != nil {
			status := dto.ErrorStatusMap[err]
			if status == 0 {
				status = 500
			}
			dto.Error(c, status, err.Error(), v)
		} else {
			dto.HandleError(c, err)
		}
		return
	}
	dto.OK(c, v)
}

// OCRResult godoc
// @Summary Get the latest or a specific OCR attempt for a receipt
// @Description Returns PROCESSING while active; an abandoned attempt past the 30-second deadline is reported FAILED with OCR_TIMEOUT. Reads do not change the Expense. The next edit or OCR request durably reaps the expired attempt.
// @Tags ocr
// @Produce json
// @Security BearerAuth
// @Param id path int true "Expense ID"
// @Param receipt_id query int true "Receipt ID"
// @Param extraction_id query int false "Specific attempt ID"
// @Success 200 {object} dto.BaseResponse{data=dto.OCRResponse}
// @Failure 400,401,403,404,500 {object} dto.BaseResponse
// @Router /api/v1/expenses/{id}/ocr-result [get]
func (h *ExpenseHandler) OCRResult(c *gin.Context) {
	a, r, id, ok := expenseIdentity(c)
	if !ok {
		return
	}
	var q dto.OCRResultQuery
	if c.ShouldBindQuery(&q) != nil {
		dto.HandleError(c, dto.ErrBadRequest)
		return
	}
	v, err := h.service.OCRResult(c.Request.Context(), a, r, id, q)
	if err != nil {
		dto.HandleError(c, err)
		return
	}
	dto.OK(c, v)
}
