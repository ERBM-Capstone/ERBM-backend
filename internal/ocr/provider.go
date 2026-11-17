// Package ocr defines the extraction-provider boundary. OCR output is never an
// authoritative money write.
package ocr

import (
	"context"
	"encoding/json"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
)

type Document struct {
	Bytes       []byte
	ContentType string
	PageCount   int
}
type Result struct {
	// PagesRead must equal the document's page count for a multipage PDF.
	PagesRead    int
	Raw          json.RawMessage
	Amount       *string
	Currency     *string
	ExpenseDate  *string
	Description  *string
	MerchantName *string
	InvoiceNo    *string
	LineItems    []dto.ExpenseLineItem
}
type Provider interface {
	Name() string
	Model() string
	// Implementations must stop work when the context is canceled. The caller
	// also discards late results and bounds the entire file to its configured timeout.
	Extract(context.Context, Document) (*Result, error)
}

// Unconfigured keeps OCR explicitly disabled until a configured Provider is
// supplied to ExpenseService. It never sends files to an external service.
type Unconfigured struct{}

func (Unconfigured) Name() string  { return "private-typhoon" }
func (Unconfigured) Model() string { return "typhoon-ocr-7b" }
func (Unconfigured) Extract(context.Context, Document) (*Result, error) {
	return nil, dto.ErrOCRUnavailable
}
