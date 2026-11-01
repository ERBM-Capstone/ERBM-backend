package model

import "time"

// AdvanceHistory reads current visibility and all workflow events together.
type AdvanceHistory struct {
	Advance AdvanceListItem
	Events  []AdvanceHistoryItem
}

// AdvanceHistoryItem excludes the internal before/after audit snapshots.
type AdvanceHistoryItem struct {
	ID              int64
	Action          AdvanceAction
	ActorUserID     int64
	ActorFirstName  string
	ActorLastName   string
	PreviousStatus  *AdvanceStatus
	ResultingStatus AdvanceStatus
	Reason          *string
	// Submission fields are nil for other actions. Content comes from the
	// immutable snapshot; revision numbers follow successful submission order.
	Revision   *int64
	Amount     *string
	Purpose    *string
	OccurredAt time.Time
}
