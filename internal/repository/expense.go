package repository

import (
	"context"
	"errors"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExpenseRepository struct{ db *gorm.DB }

func NewExpenseRepository(db *gorm.DB) *ExpenseRepository { return &ExpenseRepository{db: db} }
func (r *ExpenseRepository) conn(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = r.db
	}
	return tx.WithContext(ctx)
}
func (r *ExpenseRepository) WithTx(ctx context.Context, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}
func lock(db *gorm.DB, write bool) *gorm.DB {
	strength := "SHARE"
	if write {
		strength = "UPDATE"
	}
	return db.Clauses(clause.Locking{Strength: strength})
}

func (r *ExpenseRepository) Advance(ctx context.Context, tx *gorm.DB, id int64, locked, write bool) (*model.Advance, error) {
	db := r.conn(ctx, tx)
	if locked {
		db = lock(db, write)
	}
	var a model.Advance
	err := db.Where("id = ?", id).Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrAdvanceNotFound
	}
	return &a, err
}
func (r *ExpenseRepository) Project(ctx context.Context, tx *gorm.DB, id int64, write bool) (*model.Project, error) {
	var p model.Project
	err := lock(r.conn(ctx, tx), write).Where("id = ?", id).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrProjectNotFound
	}
	return &p, err
}
func (r *ExpenseRepository) Expense(ctx context.Context, tx *gorm.DB, id int64, locked, write bool) (*model.Expense, error) {
	db := r.conn(ctx, tx)
	if locked {
		db = lock(db, write)
	}
	var e model.Expense
	err := db.Where("id = ?", id).Take(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrExpenseNotFound
	}
	return &e, err
}
func (r *ExpenseRepository) CreateExpense(ctx context.Context, tx *gorm.DB, e *model.Expense) error {
	return r.conn(ctx, tx).Create(e).Error
}
func (r *ExpenseRepository) SaveExpense(ctx context.Context, tx *gorm.DB, e *model.Expense) error {
	res := r.conn(ctx, tx).Model(&model.Expense{}).Where("id = ? AND advance_id = ?", e.ID, e.AdvanceID).Updates(map[string]any{
		"expense_date": e.ExpenseDate, "description": e.Description, "merchant_name": e.MerchantName, "invoice_no": e.InvoiceNo,
		"amount": e.Amount, "status": e.Status, "line_items": e.LineItems, "version": e.Version,
		"confirmed_by": e.ConfirmedBy,
		"confirmed_at": e.ConfirmedAt, "updated_by": e.UpdatedBy, "updated_at": e.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return dto.ErrExpenseNotFound
	}
	return nil
}
func (r *ExpenseRepository) DeleteExpense(ctx context.Context, tx *gorm.DB, id int64) error {
	res := r.conn(ctx, tx).Where("id = ?", id).Delete(&model.Expense{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return dto.ErrExpenseNotFound
	}
	return nil
}
func (r *ExpenseRepository) ListExpenses(ctx context.Context, tx *gorm.DB, advanceID int64, page, size int) ([]model.Expense, int64, error) {
	db := r.conn(ctx, tx).Model(&model.Expense{}).Where("advance_id = ?", advanceID).Session(&gorm.Session{})
	var n int64
	if err := db.Count(&n).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.Expense, 0)
	err := db.Order("created_at DESC, id DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, n, err
}
func (r *ExpenseRepository) Totals(ctx context.Context, tx *gorm.DB, id int64) (*model.ExpenseTotals, error) {
	var t model.ExpenseTotals
	err := r.conn(ctx, tx).Raw(`SELECT COALESCE(SUM(amount) FILTER (WHERE status = 'CONFIRMED'),0)::text AS confirmed_total,
		COUNT(*) AS expense_count, COUNT(*) FILTER (WHERE status = 'CONFIRMED') AS confirmed_count,
		COUNT(*) FILTER (WHERE status = 'DRAFT') AS unconfirmed_count
		FROM expenses WHERE advance_id = ?`, id).Scan(&t).Error
	return &t, err
}
func (r *ExpenseRepository) RecordExpenseEvent(ctx context.Context, tx *gorm.DB, e *model.ExpenseAuditEvent) error {
	return r.conn(ctx, tx).Create(e).Error
}
func (r *ExpenseRepository) Documents(ctx context.Context, tx *gorm.DB, id int64) ([]model.ExpenseDocument, error) {
	d := make([]model.ExpenseDocument, 0)
	err := r.conn(ctx, tx).Where("expense_id = ?", id).Order("uploaded_at, id").Find(&d).Error
	return d, err
}
func (r *ExpenseRepository) Document(ctx context.Context, tx *gorm.DB, id, expenseID int64, write bool) (*model.ExpenseDocument, error) {
	var d model.ExpenseDocument
	err := lock(r.conn(ctx, tx), write).Where("id = ? AND expense_id = ?", id, expenseID).Take(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrReceiptNotFound
	}
	return &d, err
}
func (r *ExpenseRepository) CreateDocument(ctx context.Context, tx *gorm.DB, d *model.ExpenseDocument) error {
	return r.conn(ctx, tx).Create(d).Error
}
func (r *ExpenseRepository) DeleteDocument(ctx context.Context, tx *gorm.DB, id int64) error {
	res := r.conn(ctx, tx).Where("id = ?", id).Delete(&model.ExpenseDocument{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return dto.ErrReceiptNotFound
	}
	return nil
}
func (r *ExpenseRepository) QueueDeletion(ctx context.Context, tx *gorm.DB, d *model.ExpenseDocument, actor int64) (int64, error) {
	t := model.FileDeletionTask{StorageProvider: d.StorageProvider, StorageKey: d.StorageKey, StorageGeneration: d.StorageGeneration,
		Status: "PENDING", NextAttemptAt: time.Now().UTC(), CreatedBy: actor}
	err := r.conn(ctx, tx).Create(&t).Error
	return t.ID, err
}

// ClaimDeletion returns at most one due task. attempt_count is also the lease
// token, preventing an old worker from acknowledging a newer worker's claim.
func (r *ExpenseRepository) ClaimDeletion(ctx context.Context, ids []int64) (*model.FileDeletionTask, error) {
	var task *model.FileDeletionTask
	err := r.WithTx(ctx, func(tx *gorm.DB) error {
		db := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_attempt_at <= ?", []string{"PENDING", "PROCESSING"}, time.Now().UTC())
		if len(ids) > 0 {
			db = db.Where("id IN ?", ids)
		}
		var t model.FileDeletionTask
		if err := db.Order("next_attempt_at, id").Take(&t).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		t.Status = "PROCESSING"
		t.AttemptCount++
		t.NextAttemptAt = time.Now().UTC().Add(time.Minute)
		if err := tx.WithContext(ctx).Model(&t).Updates(map[string]any{"status": t.Status, "attempt_count": t.AttemptCount, "next_attempt_at": t.NextAttemptAt}).Error; err != nil {
			return err
		}
		task = &t
		return nil
	})
	return task, err
}
func (r *ExpenseRepository) FinishDeletion(ctx context.Context, t *model.FileDeletionTask, success bool) error {
	now := time.Now().UTC()
	updates := map[string]any{"status": "SUCCEEDED", "completed_at": now, "last_error_code": nil}
	if !success {
		updates = map[string]any{"status": "PENDING", "next_attempt_at": now.Add(time.Minute), "last_error_code": "STORAGE_DELETE_FAILED"}
	}
	return r.conn(ctx, nil).Model(&model.FileDeletionTask{}).Where("id = ? AND status = ? AND attempt_count = ?", t.ID, "PROCESSING", t.AttemptCount).Updates(updates).Error
}
func (r *ExpenseRepository) DeletionsPending(ctx context.Context, ids []int64) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	var n int64
	err := r.conn(ctx, nil).Model(&model.FileDeletionTask{}).Where("id IN ? AND status <> ?", ids, "SUCCEEDED").Count(&n).Error
	return n > 0, err
}
func (r *ExpenseRepository) DocumentKeyExists(ctx context.Context, provider, key string) (bool, error) {
	var n int64
	err := r.conn(ctx, nil).Model(&model.ExpenseDocument{}).Where("storage_provider = ? AND storage_key = ?", provider, key).Count(&n).Error
	return n > 0, err
}

// The caller holds the Expense lock, shared by OCR start/finalization and edits.
func (r *ExpenseRepository) ExpireOCR(ctx context.Context, tx *gorm.DB, expenseID int64, before time.Time) error {
	now := time.Now().UTC()
	return r.conn(ctx, tx).Model(&model.OCRExtraction{}).
		Where("document_id IN (SELECT id FROM expense_documents WHERE expense_id = ?) AND status = ? AND started_at <= ?", expenseID, model.OCRProcessing, before).
		Updates(map[string]any{"status": model.OCRFailed, "error_code": "OCR_TIMEOUT", "finished_at": now, "updated_at": now}).Error
}
func (r *ExpenseRepository) ProcessingOCR(ctx context.Context, tx *gorm.DB, expenseID int64) (*model.OCRExtraction, error) {
	var x model.OCRExtraction
	err := r.conn(ctx, tx).Table("ocr_extractions AS x").Select("x.*").
		Joins("JOIN expense_documents AS d ON d.id = x.document_id").
		Where("d.expense_id = ? AND x.status = ?", expenseID, model.OCRProcessing).Order("x.id DESC").Take(&x).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &x, err
}
func (r *ExpenseRepository) LatestOCR(ctx context.Context, tx *gorm.DB, documentID int64, status string) (*model.OCRExtraction, error) {
	db := r.conn(ctx, tx).Where("document_id = ?", documentID)
	if status != "" {
		db = db.Where("status = ?", status)
	}
	var e model.OCRExtraction
	err := db.Order("id DESC").Take(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &e, err
}
func (r *ExpenseRepository) Extraction(ctx context.Context, tx *gorm.DB, id, documentID int64) (*model.OCRExtraction, error) {
	var e model.OCRExtraction
	err := r.conn(ctx, tx).Where("id = ? AND document_id = ?", id, documentID).Take(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.ErrOCRNotFound
	}
	return &e, err
}
func (r *ExpenseRepository) CreateOCR(ctx context.Context, tx *gorm.DB, e *model.OCRExtraction) error {
	return r.conn(ctx, tx).Create(e).Error
}
func (r *ExpenseRepository) SaveOCR(ctx context.Context, tx *gorm.DB, e *model.OCRExtraction) error {
	res := r.conn(ctx, tx).Model(&model.OCRExtraction{}).Where("id = ? AND status = ?", e.ID, model.OCRProcessing).Updates(map[string]any{
		"status": e.Status, "raw_result": e.RawResult, "extracted_amount": e.ExtractedAmount, "extracted_currency": e.ExtractedCurrency,
		"error_code": e.ErrorCode, "applied_to_draft": e.AppliedToDraft, "finished_at": e.FinishedAt, "updated_at": e.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return dto.ErrOCRNotFound
	}
	return nil
}
func (r *ExpenseRepository) CreateOCRItems(ctx context.Context, tx *gorm.DB, items []model.OCRLineItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.conn(ctx, tx).Create(&items).Error
}
func (r *ExpenseRepository) OCRItems(ctx context.Context, tx *gorm.DB, id int64) ([]model.OCRLineItem, error) {
	items := make([]model.OCRLineItem, 0)
	err := r.conn(ctx, tx).Where("extraction_id = ?", id).Order("line_no").Find(&items).Error
	return items, err
}
