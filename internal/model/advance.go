package model

import "time"

type AdvanceStatus string

const (
	AdvanceStatusDraft               AdvanceStatus = "DRAFT"
	AdvanceStatusPendingPMApproval   AdvanceStatus = "PENDING_PM_APPROVAL"
	AdvanceStatusReturnedForRevision AdvanceStatus = "RETURNED_FOR_REVISION"
	AdvanceStatusApproved            AdvanceStatus = "APPROVED"
	AdvanceStatusActive              AdvanceStatus = "ACTIVE"
	AdvanceStatusClosed              AdvanceStatus = "CLOSED"
)

func (s AdvanceStatus) IsKnown() bool {
	switch s {
	case AdvanceStatusDraft, AdvanceStatusPendingPMApproval, AdvanceStatusReturnedForRevision,
		AdvanceStatusApproved, AdvanceStatusActive, AdvanceStatusClosed:
		return true
	}
	return false
}

type AdvanceAction string

const (
	AdvanceActionCreate AdvanceAction = "CREATE"
	AdvanceActionUpdate AdvanceAction = "UPDATE"
	AdvanceActionSubmit AdvanceAction = "SUBMIT"
)

// AdvanceListItem is the read model of an Advance joined with its Project.
// Amount stays an exact decimal string in THB.
type AdvanceListItem struct {
	ID          int64
	AdvanceCode string
	ProjectID   int64
	ProjectCode string
	ProjectName string
	// Project membership is read with the Advance in one database snapshot.
	// Response DTOs omit these internal authorization fields.
	PMUserID           int64  `gorm:"column:pm_user_id"`
	PCUserID           *int64 `gorm:"column:pc_user_id"`
	Purpose            string
	Amount             string
	Status             AdvanceStatus
	CreatedBy          int64
	RequesterFirstName string
	RequesterLastName  string
	SubmittedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Advance is the persisted financial request. Codes are assigned once at creation.
type Advance struct {
	ID          int64         `json:"id"`
	AdvanceCode string        `json:"advance_code"`
	ProjectID   int64         `json:"project_id"`
	Purpose     string        `json:"purpose"`
	Amount      string        `json:"amount"`
	Status      AdvanceStatus `json:"status"`
	CreatedBy   int64         `json:"created_by"`
	UpdatedBy   int64         `json:"updated_by"`
	SubmittedAt *time.Time    `json:"submitted_at"`
	SubmittedBy *int64        `json:"submitted_by"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type AdvanceWorkflowEvent struct {
	ID              int64
	AdvanceID       int64
	ActorUserID     int64
	Action          AdvanceAction
	PreviousStatus  *AdvanceStatus
	ResultingStatus AdvanceStatus
	BeforeData      *string `gorm:"type:jsonb"`
	AfterData       string  `gorm:"type:jsonb"`
	OccurredAt      time.Time
}

type AdvanceProjectAccess struct {
	ProjectID     int64
	PMUserID      int64 `gorm:"column:pm_user_id"`
	IsCoordinator bool
}

// Submission history and current visibility are read in one database snapshot.
type AdvanceSubmissionHistory struct {
	Advance     AdvanceListItem
	Submissions []AdvanceSubmission
}

type AdvanceSubmission struct {
	ActorUserID int64
	AfterData   string
	OccurredAt  time.Time
}

type AdvanceRevision struct {
	Revision    int64
	SubmittedBy int64
	SubmittedAt time.Time
	Snapshot    Advance
}

// Actor fields are assigned by the service from authentication, never DTOs.
type AdvanceFilter struct {
	ProjectID   *int64
	Status      AdvanceStatus
	Query       string
	Page        int
	PageSize    int
	ActorUserID int64
	ActorRole   string
}
