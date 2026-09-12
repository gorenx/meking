package controlplane

import "errors"

var (
	ErrInvalidAction       = errors.New("invalid control Action")
	ErrInvalidPendingInput = errors.New("invalid pending Action input")
	ErrNoPendingInput      = errors.New("Action has no pending input")
	ErrInvalidPolicy       = errors.New("invalid Control Policy")
	ErrPolicyNotFound      = errors.New("Control Policy not found")
	ErrPolicyConflict      = errors.New("Control Policy revision conflict")
	ErrActionNotInvocable  = errors.New("Action is not invocable by the requested mode")
)
