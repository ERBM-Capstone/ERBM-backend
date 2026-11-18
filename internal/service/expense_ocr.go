package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/ocr"
	"github.com/ERBM-Capstone/ERBM-backend/internal/pkg/money"
	"gorm.io/gorm"
)

func (s *ExpenseService) ocrResponse(ctx context.Context, tx *gorm.DB, x *model.OCRExtraction, cached bool) (*dto.OCRResponse, error) {
	// Report an abandoned request as timed out even before a mutation reaps it.
	// Reads do not change a CLOSED Fund or require PC write privileges.
	if x.Status == model.OCRProcessing && !time.Now().Before(x.StartedAt.Add(s.ocrTimeout)) {
		copy := *x
		deadline := x.StartedAt.Add(s.ocrTimeout)
		code := "OCR_TIMEOUT"
		copy.Status = model.OCRFailed
		copy.ErrorCode = &code
		copy.FinishedAt = &deadline
		x = &copy
	}
	items, err := s.repo.OCRItems(ctx, tx, x.ID)
	if err != nil {
		return nil, err
	}
	lines := make([]dto.ExpenseLineItem, 0, len(items))
	for _, i := range items {
		lines = append(lines, dto.ExpenseLineItem{LineNo: i.LineNo, PageNo: i.PageNo, Description: i.Description, Quantity: i.Quantity, UnitPrice: i.UnitPrice, LineTotal: i.LineTotal})
	}
	return &dto.OCRResponse{ExtractionID: x.ID, ReceiptID: x.DocumentID, Status: x.Status, ModelVersion: x.ModelVersion,
		ExtractedAmount: x.ExtractedAmount, ExtractedCurrency: x.ExtractedCurrency, Result: x.RawResult, LineItems: lines, ErrorCode: x.ErrorCode,
		Cached: cached, DraftApplied: x.AppliedToDraft && !cached, CreatedAt: x.CreatedAt, FinishedAt: x.FinishedAt}, nil
}
func (s *ExpenseService) OCRResult(ctx context.Context, actor int64, role string, id int64, q dto.OCRResultQuery) (*dto.OCRResponse, error) {
	if err := expenseActor(actor, role, false); err != nil {
		return nil, err
	}
	if q.ReceiptID <= 0 || (q.ExtractionID != nil && *q.ExtractionID <= 0) {
		return nil, dto.ErrBadRequest
	}
	var result *dto.OCRResponse
	err := s.withExpense(ctx, actor, role, id, false, func(tx *gorm.DB, _ *model.Advance, _ *model.Expense) error {
		if _, err := s.repo.Document(ctx, tx, q.ReceiptID, id, false); err != nil {
			return err
		}
		var x *model.OCRExtraction
		var err error
		if q.ExtractionID != nil {
			x, err = s.repo.Extraction(ctx, tx, *q.ExtractionID, q.ReceiptID)
		} else {
			x, err = s.repo.LatestOCR(ctx, tx, q.ReceiptID, "")
		}
		if err != nil {
			return err
		}
		if x == nil {
			return dto.ErrOCRNotFound
		}
		result, err = s.ocrResponse(ctx, tx, x, true)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *ExpenseService) ReadOCR(ctx context.Context, actor int64, role string, id int64, req *dto.ReadReceiptRequest) (*dto.OCRResponse, error) {
	if err := expenseActor(actor, role, true); err != nil {
		return nil, err
	}
	if req == nil || req.ReceiptID <= 0 {
		return nil, dto.ErrBadRequest
	}
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, s.ocrTimeout)
	defer cancel()
	var doc *model.ExpenseDocument
	var attempt *model.OCRExtraction
	var result *dto.OCRResponse
	err := s.withExpense(ctx, actor, role, id, true, func(tx *gorm.DB, _ *model.Advance, e *model.Expense) error {
		if err := s.ensureOCRIdle(ctx, tx, id); err != nil {
			return err
		}
		var err error
		doc, err = s.repo.Document(ctx, tx, req.ReceiptID, id, true)
		if err != nil {
			return err
		}
		if !req.Force {
			x, err := s.repo.LatestOCR(ctx, tx, doc.ID, model.OCRSucceeded)
			if err != nil {
				return err
			}
			if x != nil {
				result, err = s.ocrResponse(ctx, tx, x, true)
				if err == nil {
					response := dto.ToExpenseResponse(*e)
					result.Expense = &response
				}
				return err
			}
		}
		if e.Status != model.ExpenseStatusDraft {
			return dto.ErrExpenseAlreadyConfirmed
		}
		if _, disabled := s.provider.(ocr.Unconfigured); disabled {
			return dto.ErrOCRUnavailable
		}
		now := time.Now().UTC()
		modelVersion := s.provider.Model()
		attempt = &model.OCRExtraction{DocumentID: doc.ID, Provider: s.provider.Name(), ModelVersion: &modelVersion, Status: model.OCRProcessing,
			InputSHA256: doc.SHA256, RequestedBy: actor, CreatedAt: now, StartedAt: startedAt, UpdatedAt: now}
		return s.repo.CreateOCR(ctx, tx, attempt)
	})
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, dto.ErrOCRTimeout
		}
		return nil, err
	}
	if result != nil {
		return result, nil
	}
	settled := false
	cleanupCode := "OCR_INTERRUPTED"
	defer func() {
		if !settled {
			s.failOCRAttempt(actor, role, id, attempt, cleanupCode)
		}
	}()
	// No SQL transaction or row lock is held during storage/provider work.
	extracted, err := s.extractReceipt(ctx, doc)
	var publicErr error
	code := "OCR_FAILED"
	if err != nil || extracted == nil || ctx.Err() != nil {
		publicErr = dto.ErrOCRFailed
		if errors.Is(err, dto.ErrOCRUnavailable) {
			publicErr = dto.ErrOCRUnavailable
			code = "OCR_UNAVAILABLE"
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			publicErr = dto.ErrOCRTimeout
			code = "OCR_TIMEOUT"
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			code = "OCR_CANCELLED"
		}
	}
	if publicErr == nil {
		if len(extracted.Raw) == 0 {
			extracted.Raw = json.RawMessage(`{}`)
		}
		if len(extracted.Raw) > 8_000_000 || !json.Valid(extracted.Raw) {
			publicErr = dto.ErrOCRFailed
			code = "OCR_INVALID_RESULT"
		}
		for _, v := range []struct {
			v *string
			n int
		}{{extracted.Description, 10000}, {extracted.MerchantName, 255}, {extracted.InvoiceNo, 100}, {extracted.Currency, 3}} {
			if v.v != nil && utf8.RuneCountInString(*v.v) > v.n {
				publicErr = dto.ErrOCRFailed
				code = "OCR_INVALID_RESULT"
			}
		}
		if doc.PageCount > 1 && extracted.PagesRead != doc.PageCount {
			publicErr = dto.ErrOCRFailed
			code = "OCR_INCOMPLETE_PAGES"
		}
		for _, i := range extracted.LineItems {
			if i.PageNo > doc.PageCount {
				publicErr = dto.ErrOCRFailed
				code = "OCR_INVALID_ITEMS"
			}
		}
		if len(extracted.LineItems) > 1000 {
			publicErr = dto.ErrOCRFailed
			code = "OCR_INVALID_ITEMS"
		}
		if _, err := normalizeExpenseItems(extracted.LineItems); err != nil {
			publicErr = dto.ErrOCRFailed
			code = "OCR_INVALID_ITEMS"
		}
		if extracted.Amount != nil {
			amount, err := money.Normalize(*extracted.Amount)
			if err != nil {
				extracted.Amount = nil
			} else {
				extracted.Amount = &amount
			}
		}
		if extracted.Currency != nil && *extracted.Currency != "THB" {
			extracted.Amount = nil
		}
	}
	// A canceled HTTP request must still finish its durable attempt. This small
	// cleanup context is bounded and rechecks current authority and ACTIVE state.
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	finishErr := s.withExpense(finishCtx, actor, role, id, true, func(tx *gorm.DB, _ *model.Advance, e *model.Expense) error {
		if _, err := s.repo.Document(finishCtx, tx, doc.ID, id, true); err != nil {
			return err
		}
		x, err := s.repo.Extraction(finishCtx, tx, attempt.ID, doc.ID)
		if err != nil {
			return err
		}
		if x.Status != model.OCRProcessing {
			publicErr = dto.ErrOCRFailed
			if x.ErrorCode != nil && *x.ErrorCode == "OCR_TIMEOUT" {
				publicErr = dto.ErrOCRTimeout
			}
			result, err = s.ocrResponse(finishCtx, tx, x, false)
			if err == nil {
				response := dto.ToExpenseResponse(*e)
				result.Expense = &response
			}
			return err
		}
		if publicErr == nil && ctx.Err() != nil {
			publicErr = dto.ErrOCRFailed
			code = "OCR_CANCELLED"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				publicErr = dto.ErrOCRTimeout
				code = "OCR_TIMEOUT"
			}
		}
		now := time.Now().UTC()
		x.FinishedAt = &now
		x.UpdatedAt = now
		if publicErr != nil {
			x.Status = model.OCRFailed
			x.ErrorCode = &code
		} else {
			x.Status = model.OCRSucceeded
			x.RawResult = model.JSON(extracted.Raw)
			x.ExtractedAmount = extracted.Amount
			x.ExtractedCurrency = extracted.Currency
			rows := make([]model.OCRLineItem, 0, len(extracted.LineItems))
			for _, i := range extracted.LineItems {
				page := i.PageNo
				if page == 0 {
					page = 1
				}
				rows = append(rows, model.OCRLineItem{ExtractionID: x.ID, LineNo: i.LineNo, PageNo: page, Description: i.Description, Quantity: i.Quantity, UnitPrice: i.UnitPrice, LineTotal: i.LineTotal})
			}
			if err := s.repo.CreateOCRItems(finishCtx, tx, rows); err != nil {
				return err
			}
			if e.Status == model.ExpenseStatusDraft {
				before := *e
				items, err := normalizeExpenseItems(extracted.LineItems)
				if err != nil {
					return err
				}
				e.Amount = extracted.Amount
				e.LineItems = items
				if extracted.Description != nil {
					e.Description = extracted.Description
				}
				if extracted.MerchantName != nil {
					e.MerchantName = extracted.MerchantName
				}
				if extracted.InvoiceNo != nil {
					e.InvoiceNo = extracted.InvoiceNo
				}
				if extracted.ExpenseDate != nil {
					if d, err := time.Parse("2006-01-02", *extracted.ExpenseDate); err == nil {
						e.ExpenseDate = &d
					}
				}
				e.Version++
				e.UpdatedBy = actor
				e.UpdatedAt = now
				if err := s.repo.SaveExpense(finishCtx, tx, e); err != nil {
					return err
				}
				if err := s.audit(finishCtx, tx, e, actor, "OCR_APPLY", before, false); err != nil {
					return err
				}
				x.AppliedToDraft = true
			}
		}
		if err := s.repo.SaveOCR(finishCtx, tx, x); err != nil {
			return err
		}
		result, err = s.ocrResponse(finishCtx, tx, x, false)
		if err == nil {
			response := dto.ToExpenseResponse(*e)
			result.Expense = &response
		}
		return err
	})
	if finishErr != nil {
		cleanupCode = "OCR_SAVE_FAILED"
		return nil, finishErr
	}
	settled = true
	return result, publicErr
}
