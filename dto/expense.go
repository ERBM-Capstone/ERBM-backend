package dto

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
)

type CreateExpenseRequest struct {
	Description *string `json:"description" binding:"omitempty,max=10000"`
}

type ExpenseLineItem struct {
	LineNo      int     `json:"line_no"`
	PageNo      int     `json:"page_no,omitempty"`
	Description *string `json:"description"`
	Quantity    *string `json:"quantity"`
	UnitPrice   *string `json:"unit_price"`
	LineTotal   *string `json:"line_total"`
}

// Presence is explicit: omitted nullable fields stay unchanged, null clears them.
type UpdateExpenseRequest struct {
	ExpectedVersion int64              `json:"expected_version" binding:"required,min=1"`
	ExpenseDate     *string            `json:"expense_date"`
	Description     *string            `json:"description" binding:"omitempty,max=10000"`
	MerchantName    *string            `json:"merchant_name" binding:"omitempty,max=255"`
	InvoiceNo       *string            `json:"invoice_no" binding:"omitempty,max=100"`
	Amount          *string            `json:"amount"`
	LineItems       *[]ExpenseLineItem `json:"line_items"`
	fields          map[string]json.RawMessage
}

func (r *UpdateExpenseRequest) UnmarshalJSON(b []byte) error {
	type plain UpdateExpenseRequest
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = UpdateExpenseRequest(v)
	return json.Unmarshal(b, &r.fields)
}
func (r *UpdateExpenseRequest) Has(field string) bool {
	for key := range r.fields {
		if strings.EqualFold(key, field) {
			return true
		}
	}
	switch field {
	case "expense_date":
		return r.ExpenseDate != nil
	case "description":
		return r.Description != nil
	case "merchant_name":
		return r.MerchantName != nil
	case "invoice_no":
		return r.InvoiceNo != nil
	case "amount":
		return r.Amount != nil
	case "line_items":
		return r.LineItems != nil
	}
	return false
}

type ConfirmExpenseRequest struct {
	ExpectedVersion int64 `json:"expected_version" binding:"required,min=1"`
}
type ReopenExpenseRequest struct {
	ExpectedVersion int64 `json:"expected_version" binding:"required,min=1"`
}
type ReadReceiptRequest struct {
	ReceiptID int64 `json:"receipt_id" binding:"required,min=1"`
	Force     bool  `json:"force"`
}
type ReceiptURI struct {
	ID        int64 `uri:"id" binding:"required,min=1"`
	ReceiptID int64 `uri:"receipt_id" binding:"required,min=1"`
}
type OCRResultQuery struct {
	ReceiptID    int64  `form:"receipt_id" binding:"required,min=1"`
	ExtractionID *int64 `form:"extraction_id" binding:"omitempty,min=1"`
}

type ExpenseResponse struct {
	ID                int64               `json:"id"`
	AdvanceID         int64               `json:"advance_id"`
	ExpenseDate       *string             `json:"expense_date"`
	Description       *string             `json:"description"`
	MerchantName      *string             `json:"merchant_name"`
	InvoiceNo         *string             `json:"invoice_no"`
	Amount            *string             `json:"amount"`
	Status            model.ExpenseStatus `json:"status"`
	LineItems         model.JSON          `json:"line_items" swaggertype:"array,object"`
	Version           int64               `json:"version"`
	ConfirmedBy       *int64              `json:"confirmed_by"`
	ConfirmedAt       *time.Time          `json:"confirmed_at"`
	NeedsConfirmation bool                `json:"needs_confirmation"`
	CreatedBy         int64               `json:"created_by"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

func ToExpenseResponse(e model.Expense) ExpenseResponse {
	var date *string
	if e.ExpenseDate != nil {
		v := e.ExpenseDate.Format("2006-01-02")
		date = &v
	}
	items := e.LineItems
	if len(items) == 0 {
		items = model.JSON("[]")
	}
	return ExpenseResponse{ID: e.ID, AdvanceID: e.AdvanceID, ExpenseDate: date,
		Description: e.Description, MerchantName: e.MerchantName, InvoiceNo: e.InvoiceNo,
		Amount: e.Amount, Status: e.Status, LineItems: items, Version: e.Version,
		ConfirmedBy: e.ConfirmedBy,
		ConfirmedAt: e.ConfirmedAt, NeedsConfirmation: e.Status != model.ExpenseStatusConfirmed,
		CreatedBy: e.CreatedBy, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}

type ReceiptResponse struct {
	ID               int64     `json:"id"`
	ExpenseID        int64     `json:"expense_id"`
	OriginalFilename string    `json:"original_filename"`
	ContentType      string    `json:"content_type"`
	SizeBytes        int64     `json:"size_bytes"`
	PageCount        int       `json:"page_count"`
	UploadedBy       int64     `json:"uploaded_by"`
	UploadedAt       time.Time `json:"uploaded_at"`
}

func ToReceiptResponse(d model.ExpenseDocument) ReceiptResponse {
	return ReceiptResponse{d.ID, d.ExpenseID, d.OriginalFilename, d.ContentType, d.SizeBytes, d.PageCount, d.UploadedBy, d.UploadedAt}
}

type ReconciliationResponse struct {
	AdvanceID              int64  `json:"advance_id"`
	AdvanceAmount          string `json:"advance_amount"`
	TotalConfirmedExpenses string `json:"total_confirmed_expenses"`
	RemainingAmount        string `json:"remaining_amount"`
	ExpenseCount           int64  `json:"expense_count"`
	ConfirmedCount         int64  `json:"confirmed_count"`
	UnconfirmedCount       int64  `json:"unconfirmed_count"`
	NeedsConfirmationCount int64  `json:"needs_confirmation_count"`
}
type DeletionResponse struct {
	CleanupPending bool `json:"cleanup_pending"`
}
type OCRResponse struct {
	ExtractionID      int64             `json:"extraction_id"`
	ReceiptID         int64             `json:"receipt_id"`
	Status            string            `json:"status"`
	ModelVersion      *string           `json:"model_version"`
	ExtractedAmount   *string           `json:"extracted_amount"`
	ExtractedCurrency *string           `json:"extracted_currency"`
	Result            model.JSON        `json:"result" swaggertype:"object"`
	LineItems         []ExpenseLineItem `json:"line_items"`
	ErrorCode         *string           `json:"error_code"`
	Cached            bool              `json:"cached"`
	DraftApplied      bool              `json:"draft_applied"`
	Expense           *ExpenseResponse  `json:"expense,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	FinishedAt        *time.Time        `json:"finished_at"`
}
