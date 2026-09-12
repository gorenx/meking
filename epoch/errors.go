package epoch

import "errors"

var (
	// ErrInvalidEpoch identifies an Epoch, catch-up target, policy, or publication
	// request that violates the local value and monotonicity rules.
	ErrInvalidEpoch = errors.New("invalid Epoch")
	// ErrEpochNotFound means no Current Epoch exists or the requested positive
	// Epoch ID has never been published.
	ErrEpochNotFound = errors.New("Epoch not found")
	// ErrEpochConflict means Current changed after the caller fixed ExpectedEpoch.
	ErrEpochConflict = errors.New("Epoch publication conflict")
	// ErrEpochNotReady means at least one cross-context result required by the
	// publication could not be verified against the fixed Knowledge Versions.
	ErrEpochNotReady = errors.New("Epoch publication is not ready")
	// ErrEpochDataIntegrity identifies persisted Epoch rows or Current selection
	// that cannot reconstruct a valid immutable publication.
	ErrEpochDataIntegrity = errors.New("Epoch data integrity failure")
	// ErrEpochStorageBusy identifies SQLite lock contention that exceeded the
	// configured wait. Retrying must start by reading Current again.
	ErrEpochStorageBusy = errors.New("Epoch storage busy")
)
