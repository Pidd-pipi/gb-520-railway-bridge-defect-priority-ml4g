package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrDecisionLocked    = errors.New("final priority decisions are immutable")
	ErrReviewRole        = errors.New("reviewer or admin role is required to finalize a priority")
	ErrSeparationOfDuty  = errors.New("priority preparer cannot approve the same decision")
	ErrNotDecisionOwner  = errors.New("only the preparer may edit this draft decision")
	ErrDefectNotVerified = errors.New("only verified defects can receive handling advice")
	ErrInspectionOpen    = errors.New("inspection round has no conclusion yet")
	ErrMissingBridge     = errors.New("defect is not linked to a bridge asset")
	ErrMissingInspection = errors.New("no completed inspection round found for the bridge")
)
