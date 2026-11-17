package dto

import (
	"errors"
	"net/http"
)

// ── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound                   = errors.New("not found")
	ErrAlreadyDeleted             = errors.New("already deleted")
	ErrUnauthorized               = errors.New("unauthorized")
	ErrForbidden                  = errors.New("forbidden")
	ErrBadRequest                 = errors.New("bad request")
	ErrInvalidCredentials         = errors.New("invalid email or password")
	ErrAccountLocked              = errors.New("account temporarily locked")
	ErrInternalServer             = errors.New("internal server error")
	ErrEmployeeEmailExists        = errors.New("employee email already exists")
	ErrEmployeeAlreadyDeactivated = errors.New("employee already deactivated")
	ErrInvalidActivationToken     = errors.New("invalid or expired activation token")
	ErrActivationUnavailable      = errors.New("account is not awaiting activation")
	ErrActivationEmailDelivery    = errors.New("activation email could not be delivered")

	// Project resource
	ErrProjectNotFound            = errors.New("project not found")
	ErrProjectCodeExists          = errors.New("project code already exists")
	ErrProjectClosed              = errors.New("project is closed")
	ErrProjectHasUnclosedAdvances = errors.New("project has advances that are not CLOSED")
	ErrInvalidProjectManager      = errors.New("pm must be an active user with the PM role")
	ErrInvalidProjectCoordinator  = errors.New("pc must be an active user with the PC role")

	// Advance resource
	ErrAdvanceNotFound           = errors.New("advance not found")
	ErrAdvanceNotEditable        = errors.New("advance can only be edited in DRAFT or RETURNED_FOR_REVISION")
	ErrAdvanceNotSubmittable     = errors.New("advance can only be submitted in DRAFT")
	ErrAdvanceNotReviewable      = errors.New("advance can only be reviewed in PENDING_PM_APPROVAL")
	ErrAdvanceNotDisbursable     = errors.New("advance can only be disbursed in APPROVED")
	ErrProjectHasActiveAdvance   = errors.New("project already has an ACTIVE advance")
	ErrAdvanceAlreadyDisbursed   = errors.New("advance has already been disbursed")
	ErrExpenseNotFound           = errors.New("expense not found")
	ErrReceiptNotFound           = errors.New("receipt not found")
	ErrExpenseNotActive          = errors.New("expenses can only be changed while the advance is ACTIVE")
	ErrExpenseVersionConflict    = errors.New("expense draft has changed; reload before saving or confirming")
	ErrExpenseAmountRequired     = errors.New("review and enter the draft amount before confirming")
	ErrExpenseAlreadyConfirmed   = errors.New("reopen the confirmed expense before editing or starting OCR")
	ErrExpenseOverBudget         = errors.New("confirmed expenses exceed the advance amount")
	ErrReceiptLimit              = errors.New("an expense accepts at most 5 receipts and 50 MB")
	ErrFileTooLarge              = errors.New("receipt exceeds the 10 MB file limit")
	ErrUnsupportedReceipt        = errors.New("only JPEG, PNG and PDF receipts are supported")
	ErrInvalidReceipt            = errors.New("receipt is empty, corrupt, encrypted or exceeds 5 PDF pages")
	ErrOCRNotFound               = errors.New("OCR result not found")
	ErrOCRInProgress             = errors.New("OCR is processing this expense; wait before editing, confirming or reading again")
	ErrOCRUnavailable            = errors.New("private OCR integration is not configured")
	ErrOCRFailed                 = errors.New("OCR could not read this receipt")
	ErrOCRTimeout                = errors.New("OCR exceeded the 30 second file deadline")
	ErrStorageUnavailable        = errors.New("receipt storage is unavailable")
	ErrProjectPCUnclosedAdvances = errors.New("close all existing advances before changing the project PC")

	// Example resource
	ErrExampleNotFound = errors.New("example not found")
)
var (
	ErrSecretTooShort      = errors.New("jwt secret must be at least 32 bytes")
	ErrSecretsMustDiffer   = errors.New("jwt access and refresh secrets must differ")
	ErrIssuerRequired      = errors.New("jwt issuer is required")
	ErrAccessTTLRequired   = errors.New("jwt access token TTL must be positive")
	ErrRefreshTTLRequired  = errors.New("jwt refresh token TTL must be positive")
	ErrInvalidUserID       = errors.New("jwt user ID must be positive")
	ErrRoleRequired        = errors.New("jwt role is required")
	ErrInvalidAccessToken  = errors.New("invalid jwt access token")
	ErrInvalidRefreshToken = errors.New("invalid jwt refresh token")
)

// ── Error → HTTP status map ──────────────────────────────────────────────────

// ErrorStatusMap maps sentinel errors to HTTP status codes.
var ErrorStatusMap = map[error]int{
	ErrNotFound:                   http.StatusNotFound,
	ErrAlreadyDeleted:             http.StatusGone,
	ErrUnauthorized:               http.StatusUnauthorized,
	ErrForbidden:                  http.StatusForbidden,
	ErrBadRequest:                 http.StatusBadRequest,
	ErrInvalidCredentials:         http.StatusUnauthorized,
	ErrAccountLocked:              http.StatusLocked,
	ErrInternalServer:             http.StatusInternalServerError,
	ErrEmployeeEmailExists:        http.StatusConflict,
	ErrExampleNotFound:            http.StatusNotFound,
	ErrEmployeeAlreadyDeactivated: http.StatusConflict,
	ErrInvalidActivationToken:     http.StatusBadRequest,
	ErrActivationUnavailable:      http.StatusConflict,
	ErrActivationEmailDelivery:    http.StatusBadGateway,
	ErrProjectNotFound:            http.StatusNotFound,
	ErrProjectCodeExists:          http.StatusConflict,
	ErrProjectClosed:              http.StatusConflict,
	ErrProjectHasUnclosedAdvances: http.StatusConflict,
	ErrInvalidProjectManager:      http.StatusBadRequest,
	ErrInvalidProjectCoordinator:  http.StatusBadRequest,
	ErrAdvanceNotFound:            http.StatusNotFound,
	ErrAdvanceNotEditable:         http.StatusConflict,
	ErrAdvanceNotSubmittable:      http.StatusConflict,
	ErrAdvanceNotReviewable:       http.StatusConflict,
	ErrAdvanceNotDisbursable:      http.StatusConflict,
	ErrProjectHasActiveAdvance:    http.StatusConflict,
	ErrAdvanceAlreadyDisbursed:    http.StatusConflict,
	ErrExpenseNotFound:            http.StatusNotFound,
	ErrReceiptNotFound:            http.StatusNotFound,
	ErrExpenseNotActive:           http.StatusConflict,
	ErrExpenseVersionConflict:     http.StatusConflict,
	ErrExpenseAlreadyConfirmed:    http.StatusConflict,
	ErrExpenseAmountRequired:      http.StatusBadRequest,
	ErrExpenseOverBudget:          http.StatusConflict,
	ErrReceiptLimit:               http.StatusConflict,
	ErrFileTooLarge:               http.StatusRequestEntityTooLarge,
	ErrUnsupportedReceipt:         http.StatusUnsupportedMediaType,
	ErrInvalidReceipt:             http.StatusUnprocessableEntity,
	ErrOCRNotFound:                http.StatusNotFound,
	ErrOCRInProgress:              http.StatusConflict,
	ErrOCRUnavailable:             http.StatusServiceUnavailable,
	ErrOCRFailed:                  http.StatusBadGateway,
	ErrOCRTimeout:                 http.StatusGatewayTimeout,
	ErrStorageUnavailable:         http.StatusServiceUnavailable,
	ErrProjectPCUnclosedAdvances:  http.StatusConflict,
}
