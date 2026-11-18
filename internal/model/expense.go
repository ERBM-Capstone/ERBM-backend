package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// JSON keeps JSONB values as structured JSON in API/audit snapshots.
type JSON json.RawMessage

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	if !json.Valid(j) {
		return nil, errors.New("invalid JSON")
	}
	return string(j), nil
}
func (j *JSON) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], x...)
	case string:
		*j = append((*j)[:0], x...)
	default:
		return errors.New("invalid JSON database value")
	}
	return nil
}
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return []byte(j), nil
}

type ExpenseStatus string

const (
	ExpenseStatusDraft     ExpenseStatus = "DRAFT"
	ExpenseStatusConfirmed ExpenseStatus = "CONFIRMED"
)

type Expense struct {
	ID           int64         `json:"id"`
	AdvanceID    int64         `json:"advance_id"`
	ExpenseDate  *time.Time    `json:"expense_date"`
	Description  *string       `json:"description"`
	MerchantName *string       `json:"merchant_name"`
	InvoiceNo    *string       `json:"invoice_no"`
	Amount       *string       `json:"amount"`
	Status       ExpenseStatus `json:"status"`
	LineItems    JSON          `gorm:"type:jsonb" json:"line_items"`
	Version      int64         `json:"version"`
	ConfirmedBy  *int64        `json:"confirmed_by"`
	ConfirmedAt  *time.Time    `json:"confirmed_at"`
	CreatedBy    int64         `json:"created_by"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedBy    int64         `json:"updated_by"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type ExpenseAuditEvent struct {
	ID                int64
	AdvanceID         int64
	ExpenseID         *int64
	RecordedExpenseID int64
	ActorUserID       int64
	Action            string
	BeforeData        JSON `gorm:"type:jsonb"`
	AfterData         JSON `gorm:"type:jsonb"`
	OccurredAt        time.Time
}

type ExpenseDocument struct {
	ID                int64
	AdvanceID         int64
	ExpenseID         int64
	StorageProvider   string
	StorageKey        string
	StorageGeneration *string
	OriginalFilename  string
	ContentType       string
	SizeBytes         int64
	PageCount         int
	SHA256            string `gorm:"column:sha256"`
	UploadedBy        int64
	UploadedAt        time.Time
}

type FileDeletionTask struct {
	ID                int64
	StorageProvider   string
	StorageKey        string
	StorageGeneration *string
	Status            string
	AttemptCount      int
	NextAttemptAt     time.Time
	LastErrorCode     *string
	CreatedBy         int64
	CreatedAt         time.Time
	CompletedAt       *time.Time
}

type ExpenseTotals struct {
	ConfirmedTotal   string
	ExpenseCount     int64
	ConfirmedCount   int64
	UnconfirmedCount int64
}

const (
	OCRProcessing = "PROCESSING"
	OCRSucceeded  = "SUCCEEDED"
	OCRFailed     = "FAILED"
)

type OCRExtraction struct {
	ID                int64
	DocumentID        int64
	Provider          string
	ModelVersion      *string
	Status            string
	InputSHA256       string `gorm:"column:input_sha256"`
	AppliedToDraft    bool
	RawResult         JSON `gorm:"type:jsonb"`
	ExtractedAmount   *string
	ExtractedCurrency *string
	ErrorCode         *string
	RequestedBy       int64
	CreatedAt         time.Time
	StartedAt         time.Time
	FinishedAt        *time.Time
	UpdatedAt         time.Time
}
type OCRLineItem struct {
	ID           int64
	ExtractionID int64
	LineNo       int
	PageNo       int
	Description  *string
	Quantity     *string
	UnitPrice    *string
	LineTotal    *string
}
