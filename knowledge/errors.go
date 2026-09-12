package knowledge

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidChange identifies a command that violates a knowledge field,
	// identity, evidence, or batch-shape rule before it can be committed.
	ErrInvalidChange = errors.New("invalid knowledge change")

	// ErrIdentityConflict identifies an attempt to reuse an existing knowledge ID
	// or change the fixed endpoint or Subject binding of an existing identity.
	ErrIdentityConflict = errors.New("knowledge identity conflict")

	// ErrNotFound identifies an exact current or historical knowledge version that
	// does not exist. History reads never fall back to the current version.
	ErrNotFound = errors.New("knowledge version not found")

	// ErrDataIntegrity identifies persisted rows that cannot form a valid current
	// or historical knowledge object. Readers surface this instead of skipping data.
	ErrDataIntegrity = errors.New("knowledge data integrity failure")

	// ErrVersionExhausted identifies a version chain that reached SQLite's signed
	// 64-bit INTEGER limit and therefore cannot accept another immutable version.
	ErrVersionExhausted = errors.New("knowledge version exhausted")

	// ErrStorageBusy identifies SQLite lock contention that exceeded the configured
	// wait. Callers may retry it without treating it as an optimistic conflict.
	ErrStorageBusy = errors.New("knowledge storage busy")
)

// VersionConflict reports that a command was based on an obsolete or missing
// current version. Expected is supplied by the caller; Actual is the version
// selected inside the write transaction, with zero meaning that no current row
// exists.
type VersionConflict struct {
	// Object is the typed Entity, Relation, or Claim identity supplied by the command.
	Object ObjectRef
	// Expected is the version read by the caller before constructing the command.
	Expected Version
	// Actual is the current version observed while holding the SQLite write lock;
	// zero is the not-found sentinel and is never a stored version.
	Actual Version
}

// Error formats the stable conflict values without exposing adapter details.
func (e *VersionConflict) Error() string {
	return fmt.Sprintf(
		"%s %q version conflict: expected %d, actual %d",
		e.Object.objectType(),
		e.Object.ObjectID(),
		e.Expected,
		e.Actual,
	)
}
