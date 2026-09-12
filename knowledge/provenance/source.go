// Package provenance owns the identity and binding of Knowledge sources.
package provenance

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidSource = errors.New("invalid Knowledge source")

const (
	Agent      = "agent"
	Extraction = "extraction"
	Resolution = "resolution"
	Deletion   = "deletion"
	ChildZone  = "child_zone"
	Migration  = "migration"
)

// Source identifies the submission that produced or confirmed Knowledge. ID
// is its Knowledge idempotency key; ProducerID is the command or event identity
// in the producing boundary.
type Source struct {
	ID         string
	Kind       string
	ProducerID string
}

func Validate(source Source) error {
	if strings.TrimSpace(source.ID) == "" {
		return fmt.Errorf("%w: ID is required", ErrInvalidSource)
	}
	switch source.Kind {
	case Agent, Extraction, Resolution, Deletion, ChildZone, Migration:
	default:
		return fmt.Errorf("%w: unsupported kind %q", ErrInvalidSource, source.Kind)
	}
	if strings.TrimSpace(source.ProducerID) == "" {
		return fmt.Errorf("%w: ProducerID is required", ErrInvalidSource)
	}
	return nil
}

// SourceIDConflict reports an attempt to bind an immutable Source ID to a
// different Source definition.
func SourceIDConflict(sourceID string) error {
	return fmt.Errorf("%w: Source ID %q is already bound to another Source", ErrInvalidSource, sourceID)
}
