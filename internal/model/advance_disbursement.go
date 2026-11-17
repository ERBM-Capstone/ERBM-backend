package model

import "time"

// AdvanceDisbursement records the single full payment confirmed by Accounting.
// Amount is copied from the locked approved Advance as exact THB text.
type AdvanceDisbursement struct {
	ID          int64
	AdvanceID   int64
	Amount      string
	DisbursedAt time.Time
	ConfirmedBy int64
	ConfirmedAt time.Time
}
