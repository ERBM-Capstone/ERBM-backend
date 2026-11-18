package service

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/storage"
	"gorm.io/gorm"
)

func generation(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (s *ExpenseService) receiptAudit(ctx context.Context, tx *gorm.DB, e *model.Expense, d *model.ExpenseDocument, actor int64, deleted bool) error {
	b, err := json.Marshal(dto.ToReceiptResponse(*d))
	if err != nil {
		return err
	}
	action := "RECEIPT_UPLOAD"
	var before model.JSON
	after := model.JSON(b)
	if deleted {
		action = "RECEIPT_DELETE"
		before = b
		after, _ = json.Marshal(map[string]any{"deleted": true, "receipt_id": d.ID})
	}
	id := e.ID
	return s.repo.RecordExpenseEvent(ctx, tx, &model.ExpenseAuditEvent{AdvanceID: e.AdvanceID, ExpenseID: &id,
		RecordedExpenseID: e.ID, ActorUserID: actor, Action: action, BeforeData: before, AfterData: after, OccurredAt: time.Now().UTC()})
}
func (s *ExpenseService) UploadReceipt(ctx context.Context, actor int64, role string, id int64, name string, b []byte) (*model.ExpenseDocument, error) {
	// Bound staging lifetime well below the orphan recovery window.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := s.withExpense(ctx, actor, role, id, true, func(*gorm.DB, *model.Advance, *model.Expense) error { return nil }); err != nil {
		return nil, err
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 1024 || strings.ContainsAny(name, "\r\n\x00") {
		return nil, dto.ErrBadRequest
	}
	info, err := storage.Inspect(ctx, b)
	if err != nil {
		return nil, expenseError(err)
	}
	p, err := s.stores.Current()
	if err != nil {
		return nil, dto.ErrStorageUnavailable
	}
	key, err := storage.NewKey()
	if err != nil {
		return nil, dto.ErrInternalServer
	}
	d := &model.ExpenseDocument{ExpenseID: id, StorageProvider: p.Name(), StorageKey: key, OriginalFilename: name,
		ContentType: info.ContentType, SizeBytes: info.Size, PageCount: info.Pages, SHA256: info.SHA256, UploadedBy: actor, UploadedAt: time.Now().UTC()}
	gen, err := p.Put(ctx, key, b, info.ContentType)
	if gen != "" {
		d.StorageGeneration = &gen
	}
	if err != nil {
		s.discardUpload(d, actor)
		return nil, dto.ErrStorageUnavailable
	}
	err = s.withExpense(ctx, actor, role, id, true, func(tx *gorm.DB, a *model.Advance, e *model.Expense) error {
		docs, err := s.repo.Documents(ctx, tx, id)
		if err != nil {
			return err
		}
		var total int64
		for _, existing := range docs {
			total += existing.SizeBytes
		}
		if len(docs) >= 5 || total+d.SizeBytes > 50_000_000 {
			return dto.ErrReceiptLimit
		}
		d.AdvanceID = a.ID
		if err := s.repo.CreateDocument(ctx, tx, d); err != nil {
			return err
		}
		return s.receiptAudit(ctx, tx, e, d, actor, false)
	})
	if err != nil {
		s.discardUpload(d, actor)
		return nil, err
	}
	return d, nil
}
func (s *ExpenseService) discardUpload(d *model.ExpenseDocument, actor int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var task int64
	err := s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		var err error
		task, err = s.repo.QueueDeletion(ctx, tx, d, actor)
		return err
	})
	if err == nil {
		s.runDeletionBatch(ctx, []int64{task}, 1)
		return
	}
	// If the DB is down, try immediately. The maintenance sweep also discovers
	// old, unreferenced immutable keys after recovery; no bytes are left public.
	if p, err := s.stores.Get(d.StorageProvider); err == nil {
		_ = p.Delete(ctx, d.StorageKey, generation(d.StorageGeneration))
	}
}
func (s *ExpenseService) Receipts(ctx context.Context, actor int64, role string, id int64) ([]model.ExpenseDocument, error) {
	var result []model.ExpenseDocument
	err := s.withExpense(ctx, actor, role, id, false, func(tx *gorm.DB, _ *model.Advance, _ *model.Expense) error {
		var err error
		result, err = s.repo.Documents(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *ExpenseService) OpenReceipt(ctx context.Context, actor int64, role string, id, receiptID int64) (*model.ExpenseDocument, io.ReadCloser, error) {
	if receiptID <= 0 {
		return nil, nil, dto.ErrBadRequest
	}
	var d *model.ExpenseDocument
	err := s.withExpense(ctx, actor, role, id, false, func(tx *gorm.DB, _ *model.Advance, _ *model.Expense) error {
		var err error
		d, err = s.repo.Document(ctx, tx, receiptID, id, false)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	p, err := s.stores.Get(d.StorageProvider)
	if err != nil {
		return nil, nil, dto.ErrStorageUnavailable
	}
	r, err := p.Open(ctx, d.StorageKey, generation(d.StorageGeneration))
	if err != nil {
		return nil, nil, dto.ErrStorageUnavailable
	}
	return d, r, nil
}
func (s *ExpenseService) DeleteReceipt(ctx context.Context, actor int64, role string, id, receiptID int64) (bool, error) {
	if receiptID <= 0 {
		return false, dto.ErrBadRequest
	}
	var task int64
	err := s.withExpense(ctx, actor, role, id, true, func(tx *gorm.DB, _ *model.Advance, e *model.Expense) error {
		d, err := s.repo.Document(ctx, tx, receiptID, id, true)
		if err != nil {
			return err
		}
		task, err = s.repo.QueueDeletion(ctx, tx, d, actor)
		if err != nil {
			return err
		}
		if err := s.repo.DeleteDocument(ctx, tx, receiptID); err != nil {
			return err
		}
		return s.receiptAudit(ctx, tx, e, d, actor, true)
	})
	if err != nil {
		return false, err
	}
	return s.flushDeletionTasks([]int64{task}), nil
}
func (s *ExpenseService) runDeletionBatch(ctx context.Context, ids []int64, limit int) error {
	for i := 0; i < limit; i++ {
		t, err := s.repo.ClaimDeletion(ctx, ids)
		if err != nil {
			return err
		}
		if t == nil {
			return nil
		}
		p, err := s.stores.Get(t.StorageProvider)
		success := false
		if err == nil {
			op, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = p.Delete(op, t.StorageKey, generation(t.StorageGeneration))
			cancel()
			success = err == nil
		}
		if err := s.repo.FinishDeletion(ctx, t, success); err != nil {
			return err
		}
	}
	return nil
}
func (s *ExpenseService) flushDeletionTasks(ids []int64) bool {
	if len(ids) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.runDeletionBatch(ctx, ids, len(ids)); err != nil {
		return true
	}
	pending, err := s.repo.DeletionsPending(ctx, ids)
	return err != nil || pending
}

// RunMaintenance is called by the application with a bounded context. A crash
// leaves leased tasks reclaimable; expired staging recovery requires DB evidence
// that an object is unreferenced before deleting any bytes.
func (s *ExpenseService) RunMaintenance(ctx context.Context) error {
	if err := s.runDeletionBatch(ctx, nil, 100); err != nil {
		return err
	}
	for _, p := range s.stores.Providers() {
		objects, err := p.Expired(ctx, time.Now().UTC().Add(-24*time.Hour))
		if err != nil {
			return err
		}
		for _, o := range objects {
			exists, err := s.repo.DocumentKeyExists(ctx, p.Name(), o.Key)
			if err != nil {
				return err
			}
			if !exists {
				if err := p.Delete(ctx, o.Key, o.Generation); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
