package activation

import "errors"

var (
	ErrInvalidObservation  = errors.New("activation: invalid observation")
	ErrObservationConflict = errors.New("activation: observation identity already has different content")
	ErrTargetNotFound      = errors.New("activation: current target is missing or deleted")
	ErrVersionChanged      = errors.New("activation: current target version changed")
	ErrProtocolUnsupported = errors.New("activation: evaluation protocol is not supported")
	ErrOutOfOrder          = errors.New("activation: observation predates last applied recall")
	ErrModelMismatch       = errors.New("activation: stored state uses a different model")
	ErrDataIntegrity       = errors.New("activation: stored data violates invariants")
	ErrStorageUnavailable  = errors.New("activation: storage unavailable")
)
