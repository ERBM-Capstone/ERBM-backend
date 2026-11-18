package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/ocr"
	"github.com/ERBM-Capstone/ERBM-backend/internal/storage"
	"gorm.io/gorm"
)

const defaultReceiptOCRTimeout = 30 * time.Second

// Every caller holds the same Project -> Advance -> Expense write locks. The
// PROCESSING attempt is the durable lock; no SQL lock spans the provider call.
// Expiring abandoned attempts lets a valid edit/retry recover after a crash.
func (s *ExpenseService) ensureOCRIdle(ctx context.Context, tx *gorm.DB, expenseID int64) error {
	if err := s.repo.ExpireOCR(ctx, tx, expenseID, time.Now().UTC().Add(-s.ocrTimeout)); err != nil {
		return err
	}
	x, err := s.repo.ProcessingOCR(ctx, tx, expenseID)
	if err != nil {
		return err
	}
	if x != nil {
		return dto.ErrOCRInProgress
	}
	return nil
}

// A storage/provider failure, panic or canceled HTTP request must not strand
// the durable lock. A buffered channel also discards a late provider result.
func (s *ExpenseService) extractReceipt(ctx context.Context, d *model.ExpenseDocument) (*ocr.Result, error) {
	type outcome struct {
		result *ocr.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		v := outcome{}
		defer func() {
			if recover() != nil {
				v = outcome{err: dto.ErrOCRFailed}
			}
			done <- v
		}()
		p, err := s.stores.Get(d.StorageProvider)
		if err != nil {
			v.err = err
			return
		}
		r, err := p.Open(ctx, d.StorageKey, generation(d.StorageGeneration))
		if err != nil {
			v.err = err
			return
		}
		defer r.Close()
		b, err := storage.ReadBounded(r)
		if err != nil {
			v.err = err
			return
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != d.SHA256 {
			v.err = dto.ErrOCRFailed
			return
		}
		if err := ctx.Err(); err != nil {
			v.err = err
			return
		}
		v.result, v.err = s.provider.Extract(ctx, ocr.Document{Bytes: b, ContentType: d.ContentType, PageCount: d.PageCount})
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return v.result, v.err
	}
}

// Called after a rolled-back result transaction or an unexpected panic. It
// only terminates this attempt, never changes the Expense or a newer attempt.
// If the DB is unavailable, ensureOCRIdle reclaims the expired lock on recovery.
func (s *ExpenseService) failOCRAttempt(actor int64, role string, expenseID int64, x *model.OCRExtraction, code string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.withExpense(ctx, actor, role, expenseID, true, func(tx *gorm.DB, _ *model.Advance, _ *model.Expense) error {
		stored, err := s.repo.Extraction(ctx, tx, x.ID, x.DocumentID)
		if err != nil {
			return err
		}
		if stored.Status != model.OCRProcessing {
			return nil
		}
		now := time.Now().UTC()
		stored.Status = model.OCRFailed
		stored.ErrorCode = &code
		stored.FinishedAt = &now
		stored.UpdatedAt = now
		return s.repo.SaveOCR(ctx, tx, stored)
	})
}
