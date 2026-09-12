package journal

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/memoria-space/meking/zone"
)

type (
	ZoneID         = zone.ID
	EventID        string
	CorrelationID  string
	EventType      string
	StreamID       string
	ConsumerID     string
	Position       uint64
	EventSequence  uint64
	StreamSequence uint64
	SchemaVersion  uint32
)

// EventKey identifies one exact producer-owned body contract.
type EventKey struct {
	Type    EventType
	Version SchemaVersion
}

// ProposedEvent is the immutable producer input before Journal assigns order.
type ProposedEvent struct {
	EventID       EventID
	ZoneID        ZoneID
	StreamID      StreamID
	Type          EventType
	SchemaVersion SchemaVersion
	OccurredAt    time.Time
	CorrelationID CorrelationID
	CausationID   EventID
	Body          string
}

// Event is one immutable integration fact in an EventType sequence.
type Event struct {
	ProposedEvent
	Sequence       EventSequence
	StreamSequence StreamSequence
}

func (event Event) Key() EventKey {
	return EventKey{
		Type:    event.Type,
		Version: event.SchemaVersion,
	}
}

// TrimThrough returns the prefix before the exclusive consumer position.
func TrimThrough(events []Event, position Position) []Event {
	for index, event := range events {
		if Position(event.Sequence) >= position {
			return events[:index]
		}
	}
	return events
}

// EventContract validates the envelope, exact version, and typed body before publication.
type EventContract interface {
	ValidateAndCanonicalize(event ProposedEvent) (ProposedEvent, error)
}

func validateZoneID(id ZoneID) error {
	if _, err := zone.ParseID(string(id)); err != nil {
		return fmt.Errorf("%w: ZoneID is invalid: %v", ErrInvalidEvent, err)
	}
	return nil
}

func validateEventID(id EventID, field string, optional bool) error {
	if optional && id == "" {
		return nil
	}
	return validateIdentity(string(id), field)
}

func validateIdentity(value string, field string) error {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || len(value) > 512 {
		return fmt.Errorf("%w: %s is invalid", ErrInvalidEvent, field)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%w: %s is invalid", ErrInvalidEvent, field)
		}
	}
	return nil
}

func validateEventKey(key EventKey) error {
	if err := validateIdentity(string(key.Type), "EventType"); err != nil {
		return err
	}
	if key.Version == 0 {
		return fmt.Errorf("%w: SchemaVersion must be positive", ErrInvalidEvent)
	}
	return nil
}
