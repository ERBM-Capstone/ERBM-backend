package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	"github.com/ERBM-Capstone/ERBM-backend/internal/ocr"
	"github.com/ERBM-Capstone/ERBM-backend/internal/pkg/money"
	"github.com/ERBM-Capstone/ERBM-backend/internal/pkg/pagination"
	"github.com/ERBM-Capstone/ERBM-backend/internal/storage"
	"gorm.io/gorm"
)

type expenseRepository interface {
	WithTx(context.Context, func(*gorm.DB) error) error
	Advance(context.Context, *gorm.DB, int64, bool, bool) (*model.Advance, error)
	Project(context.Context, *gorm.DB, int64, bool) (*model.Project, error)
	Expense(context.Context, *gorm.DB, int64, bool, bool) (*model.Expense, error)
	CreateExpense(context.Context, *gorm.DB, *model.Expense) error
	SaveExpense(context.Context, *gorm.DB, *model.Expense) error
	DeleteExpense(context.Context, *gorm.DB, int64) error
	ListExpenses(context.Context, *gorm.DB, int64, int, int) ([]model.Expense, int64, error)
	Totals(context.Context, *gorm.DB, int64) (*model.ExpenseTotals, error)
	RecordExpenseEvent(context.Context, *gorm.DB, *model.ExpenseAuditEvent) error
	Documents(context.Context, *gorm.DB, int64) ([]model.ExpenseDocument, error)
	Document(context.Context, *gorm.DB, int64, int64, bool) (*model.ExpenseDocument, error)
	CreateDocument(context.Context, *gorm.DB, *model.ExpenseDocument) error
	DeleteDocument(context.Context, *gorm.DB, int64) error
	QueueDeletion(context.Context, *gorm.DB, *model.ExpenseDocument, int64) (int64, error)
	ClaimDeletion(context.Context, []int64) (*model.FileDeletionTask, error)
	FinishDeletion(context.Context, *model.FileDeletionTask, bool) error
	DeletionsPending(context.Context, []int64) (bool, error)
	DocumentKeyExists(context.Context, string, string) (bool, error)
	ExpireOCR(context.Context, *gorm.DB, int64, time.Time) error
	ProcessingOCR(context.Context, *gorm.DB, int64) (*model.OCRExtraction, error)
	LatestOCR(context.Context, *gorm.DB, int64, string) (*model.OCRExtraction, error)
	Extraction(context.Context, *gorm.DB, int64, int64) (*model.OCRExtraction, error)
	CreateOCR(context.Context, *gorm.DB, *model.OCRExtraction) error
	SaveOCR(context.Context, *gorm.DB, *model.OCRExtraction) error
	CreateOCRItems(context.Context, *gorm.DB, []model.OCRLineItem) error
	OCRItems(context.Context, *gorm.DB, int64) ([]model.OCRLineItem, error)
}

type ExpenseOptions struct {
	Storage *storage.Manager
	OCR     ocr.Provider
	// OCRTimeout bounds the entire file and its PROCESSING lock lifetime.
	// Non-positive values use the default 30-second timeout.
	OCRTimeout time.Duration
}
type ExpenseService struct {
	repo       expenseRepository
	stores     *storage.Manager
	provider   ocr.Provider
	ocrTimeout time.Duration
}

func NewExpenseService(repo expenseRepository, opts ExpenseOptions) *ExpenseService {
	p := opts.OCR
	if p == nil {
		p = ocr.Unconfigured{}
	}
	timeout := opts.OCRTimeout
	if timeout <= 0 {
		timeout = defaultReceiptOCRTimeout
	}
	return &ExpenseService{repo: repo, stores: opts.Storage, provider: p, ocrTimeout: timeout}
}

func expenseError(err error) error {
	if err == nil {
		return nil
	}
	for known := range dto.ErrorStatusMap {
		if errors.Is(err, known) {
			return known
		}
	}
	return dto.ErrInternalServer
}
func expenseActor(actor int64, role string, write bool) error {
	if actor <= 0 {
		return dto.ErrUnauthorized
	}
	role = strings.ToUpper(strings.TrimSpace(role))
	if write {
		if role != rolePC {
			return dto.ErrForbidden
		}
		return nil
	}
	switch role {
	case rolePC, rolePM, roleAdmin, roleAccounting:
		return nil
	}
	return dto.ErrForbidden
}
func (s *ExpenseService) withAdvance(ctx context.Context, actor int64, role string, id int64, write bool, fn func(*gorm.DB, *model.Advance) error) error {
	if err := expenseActor(actor, role, write); err != nil {
		return err
	}
	if id <= 0 {
		return dto.ErrBadRequest
	}
	parent, err := s.repo.Advance(ctx, nil, id, false, false)
	if err != nil {
		return expenseError(err)
	}
	err = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		p, err := s.repo.Project(ctx, tx, parent.ProjectID, write)
		if err != nil {
			return err
		}
		a, err := s.repo.Advance(ctx, tx, id, true, write)
		if err != nil {
			return err
		}
		role = strings.ToUpper(strings.TrimSpace(role))
		if (role == rolePC && (p.PCUserID == nil || *p.PCUserID != actor)) || (role == rolePM && p.PMUserID != actor) {
			return dto.ErrForbidden
		}
		if a.Status == model.AdvanceStatusDraft && a.CreatedBy != actor {
			return dto.ErrForbidden
		}
		if write {
			if a.CreatedBy != actor {
				return dto.ErrForbidden
			}
			if p.Status == model.ProjectStatusClosed {
				return dto.ErrProjectClosed
			}
			if a.Status != model.AdvanceStatusActive {
				return dto.ErrExpenseNotActive
			}
		}
		return fn(tx, a)
	})
	return expenseError(err)
}
func (s *ExpenseService) withExpense(ctx context.Context, actor int64, role string, id int64, write bool, fn func(*gorm.DB, *model.Advance, *model.Expense) error) error {
	if err := expenseActor(actor, role, write); err != nil {
		return err
	}
	if id <= 0 {
		return dto.ErrBadRequest
	}
	parent, err := s.repo.Expense(ctx, nil, id, false, false)
	if err != nil {
		return expenseError(err)
	}
	return s.withAdvance(ctx, actor, role, parent.AdvanceID, write, func(tx *gorm.DB, a *model.Advance) error {
		e, err := s.repo.Expense(ctx, tx, id, true, write)
		if err != nil {
			return err
		}
		if e.AdvanceID != a.ID {
			return dto.ErrExpenseNotFound
		}
		return fn(tx, a, e)
	})
}
func (s *ExpenseService) audit(ctx context.Context, tx *gorm.DB, e *model.Expense, actor int64, action string, before any, deleted bool) error {
	var old model.JSON
	if before != nil {
		b, err := json.Marshal(before)
		if err != nil {
			return err
		}
		old = b
	}
	var next any = e
	if deleted {
		next = map[string]any{"deleted": true, "expense_id": e.ID}
	}
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	id := e.ID
	ref := &id
	if deleted {
		ref = nil
	}
	return s.repo.RecordExpenseEvent(ctx, tx, &model.ExpenseAuditEvent{AdvanceID: e.AdvanceID, ExpenseID: ref,
		RecordedExpenseID: e.ID, ActorUserID: actor, Action: action, BeforeData: old, AfterData: b, OccurredAt: time.Now().UTC()})
}
func (s *ExpenseService) Create(ctx context.Context, actor int64, role string, advanceID int64, req *dto.CreateExpenseRequest) (*model.Expense, error) {
	if err := expenseActor(actor, role, true); err != nil {
		return nil, err
	}
	if req == nil || (req.Description != nil && utf8.RuneCountInString(*req.Description) > 10000) {
		return nil, dto.ErrBadRequest
	}
	var result *model.Expense
	err := s.withAdvance(ctx, actor, role, advanceID, true, func(tx *gorm.DB, a *model.Advance) error {
		now := time.Now().UTC()
		e := &model.Expense{AdvanceID: a.ID, Description: req.Description, Status: model.ExpenseStatusDraft, LineItems: model.JSON("[]"), Version: 1,
			CreatedBy: actor, UpdatedBy: actor, CreatedAt: now, UpdatedAt: now}
		if err := s.repo.CreateExpense(ctx, tx, e); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, e, actor, "CREATE", nil, false); err != nil {
			return err
		}
		result = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *ExpenseService) Get(ctx context.Context, actor int64, role string, id int64) (*model.Expense, error) {
	var result *model.Expense
	err := s.withExpense(ctx, actor, role, id, false, func(_ *gorm.DB, _ *model.Advance, e *model.Expense) error { result = e; return nil })
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *ExpenseService) List(ctx context.Context, actor int64, role string, id int64, page, size int) ([]model.Expense, int64, error) {
	if err := expenseActor(actor, role, false); err != nil {
		return nil, 0, err
	}
	page, size = pagination.WithDefaults(page, size)
	if !pagination.IsValid(page, size) {
		return nil, 0, dto.ErrBadRequest
	}
	var result []model.Expense
	var n int64
	err := s.withAdvance(ctx, actor, role, id, false, func(tx *gorm.DB, _ *model.Advance) error {
		var err error
		result, n, err = s.repo.ListExpenses(ctx, tx, id, page, size)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return result, n, nil
}

var signedDecimal = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

func lineDecimal(v *string, wholeMax, scale int) (*string, error) {
	if v == nil {
		return nil, nil
	}
	x := strings.TrimSpace(*v)
	if !signedDecimal.MatchString(x) {
		return nil, dto.ErrBadRequest
	}
	neg := strings.HasPrefix(x, "-")
	x = strings.TrimPrefix(x, "-")
	w, f, _ := strings.Cut(x, ".")
	if len(w) > wholeMax || len(f) > scale {
		return nil, dto.ErrBadRequest
	}
	w = strings.TrimLeft(w, "0")
	if w == "" {
		w = "0"
	}
	f = strings.TrimRight(f, "0")
	x = w
	if f != "" {
		x += "." + f
	}
	if neg && x != "0" {
		x = "-" + x
	}
	return &x, nil
}
func normalizeExpenseItems(items []dto.ExpenseLineItem) (model.JSON, error) {
	if len(items) > 1000 {
		return nil, dto.ErrBadRequest
	}
	for i := range items {
		v := &items[i]
		if v.LineNo != i+1 || v.PageNo < 0 || v.PageNo > 5 || (v.Description != nil && utf8.RuneCountInString(*v.Description) > 10000) {
			return nil, dto.ErrBadRequest
		}
		var err error
		v.Quantity, err = lineDecimal(v.Quantity, 12, 6)
		if err != nil {
			return nil, err
		}
		v.UnitPrice, err = lineDecimal(v.UnitPrice, 13, 2)
		if err != nil {
			return nil, err
		}
		v.LineTotal, err = lineDecimal(v.LineTotal, 13, 2)
		if err != nil {
			return nil, err
		}
	}
	if items == nil {
		items = []dto.ExpenseLineItem{}
	}
	b, err := json.Marshal(items)
	return b, err
}
func (s *ExpenseService) Update(ctx context.Context, actor int64, role string, id int64, req *dto.UpdateExpenseRequest) (*model.Expense, error) {
	if err := expenseActor(actor, role, true); err != nil {
		return nil, err
	}
	if req == nil || req.ExpectedVersion <= 0 {
		return nil, dto.ErrBadRequest
	}
	updates := map[string]bool{}
	for _, f := range []string{"expense_date", "description", "merchant_name", "invoice_no", "amount", "line_items"} {
		if req.Has(f) {
			updates[f] = true
		}
	}
	if len(updates) == 0 {
		return nil, dto.ErrBadRequest
	}
	var date *time.Time
	if req.ExpenseDate != nil {
		d, err := time.Parse("2006-01-02", *req.ExpenseDate)
		if err != nil {
			return nil, dto.ErrBadRequest
		}
		date = &d
	}
	for _, v := range []struct {
		v *string
		n int
	}{{req.Description, 10000}, {req.MerchantName, 255}, {req.InvoiceNo, 100}} {
		if v.v != nil && utf8.RuneCountInString(*v.v) > v.n {
			return nil, dto.ErrBadRequest
		}
	}
	var amount *string
	if req.Amount != nil {
		v, err := money.Normalize(*req.Amount)
		if err != nil {
			return nil, dto.ErrBadRequest
		}
		amount = &v
	}
	var items model.JSON
	if updates["line_items"] {
		if req.LineItems == nil {
			return nil, dto.ErrBadRequest
		}
		var err error
		items, err = normalizeExpenseItems(append([]dto.ExpenseLineItem{}, (*req.LineItems)...))
		if err != nil {
			return nil, err
		}
	}
	var result *model.Expense
	err := s.withExpense(ctx, actor, role, id, true, func(tx *gorm.DB, _ *model.Advance, e *model.Expense) error {
		if err := s.ensureOCRIdle(ctx, tx, id); err != nil {
			return err
		}
		if e.Version != req.ExpectedVersion {
			return dto.ErrExpenseVersionConflict
		}
		if e.Status != model.ExpenseStatusDraft {
			return dto.ErrExpenseAlreadyConfirmed
		}
		before := *e
		if updates["expense_date"] {
			e.ExpenseDate = date
		}
		if updates["description"] {
			e.Description = req.Description
		}
		if updates["merchant_name"] {
			e.MerchantName = req.MerchantName
		}
		if updates["invoice_no"] {
			e.InvoiceNo = req.InvoiceNo
		}
		if updates["amount"] {
			e.Amount = amount
		}
		if updates["line_items"] {
			e.LineItems = items
		}
		e.Version++
		e.UpdatedBy = actor
		e.UpdatedAt = time.Now().UTC()
		if err := s.repo.SaveExpense(ctx, tx, e); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, e, actor, "UPDATE", before, false); err != nil {
			return err
		}
		result = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *ExpenseService) Confirm(ctx context.Context, actor int64, role string, id int64, req *dto.ConfirmExpenseRequest) (*model.Expense, error) {
	if err := expenseActor(actor, role, true); err != nil {
		return nil, err
	}
	if req == nil || req.ExpectedVersion <= 0 {
		return nil, dto.ErrBadRequest
	}
	var result *model.Expense
	err := s.withExpense(ctx, actor, role, id, true, func(tx *gorm.DB, a *model.Advance, e *model.Expense) error {
		if err := s.ensureOCRIdle(ctx, tx, id); err != nil {
			return err
		}
		if e.Version != req.ExpectedVersion {
			return dto.ErrExpenseVersionConflict
		}
		if e.Status == model.ExpenseStatusConfirmed {
			result = e
			return nil
		}
		if e.Amount == nil {
			return dto.ErrExpenseAmountRequired
		}
		value, err := money.Normalize(*e.Amount)
		if err != nil {
			return dto.ErrBadRequest
		}
		totals, err := s.repo.Totals(ctx, tx, a.ID)
		if err != nil {
			return err
		}
		next, err := money.Replace(totals.ConfirmedTotal, "0", value)
		if err != nil {
			return err
		}
		cmp, err := money.Compare(next, a.Amount)
		if err != nil {
			return err
		}
		if cmp > 0 {
			return dto.ErrExpenseOverBudget
		}
		before := *e
		now := time.Now().UTC()
		e.Status = model.ExpenseStatusConfirmed
		e.ConfirmedBy = &actor
		e.ConfirmedAt = &now
		e.UpdatedBy = actor
		e.UpdatedAt = now
		if err := s.repo.SaveExpense(ctx, tx, e); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, e, actor, "CONFIRM", before, false); err != nil {
			return err
		}
		result = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *ExpenseService) Reconciliation(ctx context.Context, actor int64, role string, id int64) (*dto.ReconciliationResponse, error) {
	var result *dto.ReconciliationResponse
	err := s.withAdvance(ctx, actor, role, id, false, func(tx *gorm.DB, a *model.Advance) error {
		t, err := s.repo.Totals(ctx, tx, id)
		if err != nil {
			return err
		}
		total, err := money.Replace(t.ConfirmedTotal, "0", "0")
		if err != nil {
			return err
		}
		remaining, err := money.Subtract(a.Amount, total)
		if err != nil {
			return err
		}
		result = &dto.ReconciliationResponse{AdvanceID: id, AdvanceAmount: a.Amount, TotalConfirmedExpenses: total, RemainingAmount: remaining,
			ExpenseCount: t.ExpenseCount, ConfirmedCount: t.ConfirmedCount, UnconfirmedCount: t.UnconfirmedCount, NeedsConfirmationCount: t.UnconfirmedCount}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Reopen is the PC's explicit decision to withdraw confirmation. Keeping the
// amount lets the PC correct it; DRAFT status removes it from actual totals.
func (s *ExpenseService) Reopen(ctx context.Context, actor int64, role string, id int64, req *dto.ReopenExpenseRequest) (*model.Expense, error) {
	if err := expenseActor(actor, role, true); err != nil {
		return nil, err
	}
	if req == nil || req.ExpectedVersion <= 0 {
		return nil, dto.ErrBadRequest
	}
	var result *model.Expense
	err := s.withExpense(ctx, actor, role, id, true, func(tx *gorm.DB, _ *model.Advance, e *model.Expense) error {
		if err := s.ensureOCRIdle(ctx, tx, id); err != nil {
			return err
		}
		if e.Version != req.ExpectedVersion {
			return dto.ErrExpenseVersionConflict
		}
		if e.Status == model.ExpenseStatusDraft {
			result = e
			return nil
		}
		before := *e
		e.Status = model.ExpenseStatusDraft
		e.ConfirmedBy = nil
		e.ConfirmedAt = nil
		// Invalidate forms loaded before confirmation was withdrawn.
		e.Version++
		e.UpdatedBy = actor
		e.UpdatedAt = time.Now().UTC()
		if err := s.repo.SaveExpense(ctx, tx, e); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, e, actor, "REOPEN", before, false); err != nil {
			return err
		}
		result = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *ExpenseService) Delete(ctx context.Context, actor int64, role string, id int64) (bool, error) {
	var tasks []int64
	err := s.withExpense(ctx, actor, role, id, true, func(tx *gorm.DB, _ *model.Advance, e *model.Expense) error {
		docs, err := s.repo.Documents(ctx, tx, id)
		if err != nil {
			return err
		}
		for i := range docs {
			task, err := s.repo.QueueDeletion(ctx, tx, &docs[i], actor)
			if err != nil {
				return err
			}
			tasks = append(tasks, task)
		}
		if err := s.repo.DeleteExpense(ctx, tx, id); err != nil {
			return err
		}
		return s.audit(ctx, tx, e, actor, "DELETE", *e, true)
	})
	if err != nil {
		return false, err
	}
	return s.flushDeletionTasks(tasks), nil
}
