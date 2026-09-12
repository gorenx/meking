package journal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// EventBody supplies the protocol identity owned by one typed event body.
type EventBody interface {
	EventType() string
	SchemaVersion() uint32
}

func EventKeyFor[T EventBody]() EventKey {
	var body T
	return EventKey{
		Type:    EventType(body.EventType()),
		Version: SchemaVersion(body.SchemaVersion()),
	}
}

func JSONEventRegistration[T EventBody]() EventContractRegistration {
	return EventContractRegistration{
		Key:      EventKeyFor[T](),
		Contract: JSONEventContract[T]{},
	}
}

// JSONEventContract binds one concrete event body type to strict canonical JSON.
type JSONEventContract[T EventBody] struct{}

type bodyValidator interface {
	Validate() error
}

type emptyEventBody struct{}

func (emptyEventBody) EventType() string     { return "journal.empty" }
func (emptyEventBody) SchemaVersion() uint32 { return 1 }

var _ EventContract = JSONEventContract[emptyEventBody]{}

func (contract JSONEventContract[T]) ValidateAndCanonicalize(
	event ProposedEvent,
) (ProposedEvent, error) {
	expected := EventKeyFor[T]()
	if event.Type != expected.Type || event.SchemaVersion != expected.Version {
		return ProposedEvent{}, fmt.Errorf(
			"%w: event contract is %s v%d, got %s v%d",
			ErrInvalidEvent,
			expected.Type,
			expected.Version,
			event.Type,
			event.SchemaVersion,
		)
	}
	if err := validateZoneID(event.ZoneID); err != nil {
		return ProposedEvent{}, err
	}
	if err := validateEventID(event.EventID, "EventID", false); err != nil {
		return ProposedEvent{}, err
	}
	if err := validateIdentity(string(event.StreamID), "StreamID"); err != nil {
		return ProposedEvent{}, err
	}
	if err := validateIdentity(string(event.CorrelationID), "CorrelationID"); err != nil {
		return ProposedEvent{}, err
	}
	if err := validateEventID(event.CausationID, "CausationID", true); err != nil {
		return ProposedEvent{}, err
	}
	if event.OccurredAt.IsZero() {
		return ProposedEvent{}, fmt.Errorf("%w: OccurredAt is required", ErrInvalidEvent)
	}

	body, err := decodeJSONBody[T](event.Body)
	if err != nil {
		return ProposedEvent{}, fmt.Errorf("%w: decode %s v%d body: %v", ErrInvalidEvent, expected.Type, expected.Version, err)
	}
	canonical, err := json.Marshal(body)
	if err != nil {
		return ProposedEvent{}, fmt.Errorf("%w: encode canonical %s v%d body: %v", ErrInvalidEvent, expected.Type, expected.Version, err)
	}
	event.OccurredAt = event.OccurredAt.UTC().Round(0)
	event.Body = string(canonical)
	return event, nil
}

// DecodeJSONBody strictly decodes the exact typed body registered by a Consumer.
func DecodeJSONBody[T EventBody](event Event) (T, error) {
	expected := EventKeyFor[T]()
	if event.Type != expected.Type || event.SchemaVersion != expected.Version {
		var zero T
		return zero, fmt.Errorf(
			"%w: event body is %s v%d, got %s v%d",
			ErrInvalidEvent,
			expected.Type,
			expected.Version,
			event.Type,
			event.SchemaVersion,
		)
	}
	body, err := decodeJSONBody[T](event.Body)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("%w: decode %s v%d body: %v", ErrInvalidEvent, expected.Type, expected.Version, err)
	}
	return body, nil
}

func decodeJSONBody[T EventBody](raw string) (T, error) {
	var body T
	rawBody := bytes.TrimSpace([]byte(raw))
	if len(rawBody) == 0 || rawBody[0] != '{' {
		return body, errors.New("body must be one JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return body, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return body, errors.New("body contains more than one JSON value")
		}
		return body, fmt.Errorf("decode trailing body content: %w", err)
	}
	if validator, ok := any(body).(bodyValidator); ok {
		if err := validator.Validate(); err != nil {
			return body, err
		}
	}
	return body, nil
}
