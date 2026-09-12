package journal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/zone"
)

const (
	testZoneID      ZoneID        = "11111111-1111-4111-8111-111111111111"
	testEventID     EventID       = "22222222-2222-4222-8222-222222222222"
	testCausationID EventID       = "33333333-3333-4333-8333-333333333333"
	testCorrelation CorrelationID = "request/44444444-4444-4444-8444-444444444444"
	testEventTime                 = "2026-07-31T12:00:00-07:00"
)

type contractTestBody struct {
	Value string `json:"value"`
}

func (contractTestBody) EventType() string     { return "test.value_recorded" }
func (contractTestBody) SchemaVersion() uint32 { return 1 }
func (body contractTestBody) Validate() error {
	if body.Value == "" {
		return errors.New("value is required")
	}
	return nil
}

type contractCompletionBody struct {
	Count uint64 `json:"count"`
}

func (contractCompletionBody) EventType() string     { return "test.completed" }
func (contractCompletionBody) SchemaVersion() uint32 { return 1 }

type unregisteredBody struct{}

func (unregisteredBody) EventType() string     { return "test.unregistered" }
func (unregisteredBody) SchemaVersion() uint32 { return 1 }

func TestServiceCanonicalizesRegisteredJSONEvent(t *testing.T) {
	service := newContractTestService(t)
	occurredAt, err := time.Parse(time.RFC3339, testEventTime)
	if err != nil {
		t.Fatal(err)
	}
	event := ProposedEvent{
		EventID:       testEventID,
		ZoneID:        testZoneID,
		StreamID:      "test/stream",
		Type:          EventType(contractTestBody{}.EventType()),
		SchemaVersion: 1,
		OccurredAt:    occurredAt,
		CorrelationID: testCorrelation,
		CausationID:   testCausationID,
		Body:          " { \n \"value\" : \"kept\" } ",
	}

	canonical, err := service.canonicalize([]ProposedEvent{event, completionEvent(t)})
	if err != nil {
		t.Fatalf("canonicalize() error = %v", err)
	}
	if got := canonical[0].Body; got != `{"value":"kept"}` {
		t.Fatalf("canonical body = %q", got)
	}
	if got := canonical[0].OccurredAt.Location(); got != time.UTC {
		t.Fatalf("OccurredAt location = %v, want UTC", got)
	}

	event.Body = `{"value":"kept","unexpected":true}`
	if _, err := service.canonicalize([]ProposedEvent{event, completionEvent(t)}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("unknown body field error = %v, want ErrInvalidEvent", err)
	}
}

func TestServiceValidatesRegistrationsAndAtomicBatch(t *testing.T) {
	registrations := []EventContractRegistration{
		JSONEventRegistration[contractTestBody](),
		JSONEventRegistration[contractCompletionBody](),
	}
	atomic := AtomicBatchContract{
		MemberEvents:    []EventKey{EventKeyFor[contractTestBody]()},
		CompletionEvent: EventKeyFor[contractCompletionBody](),
	}
	if _, err := NewService(ServiceDependencies{
		Contracts: append(
			append([]EventContractRegistration(nil), registrations...),
			registrations[0],
		),
		AtomicBatches: []AtomicBatchContract{atomic},
		Reader:        contractTestReader{},
	}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("duplicate contract error = %v, want ErrInvalidRegistration", err)
	}

	service := newContractTestService(t)
	member := valueEvent(t)
	if _, err := service.canonicalize([]ProposedEvent{member}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("member without completion error = %v, want ErrInvalidEvent", err)
	}
	completion := completionEvent(t)
	if _, err := service.canonicalize([]ProposedEvent{completion, member}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("non-final completion error = %v, want ErrInvalidEvent", err)
	}
}

func TestDispatcherValidatesConsumerRegistrations(t *testing.T) {
	service := newContractTestService(t)
	positions := &contractTestPositions{}
	consumer := contractTestConsumer{
		registration: ConsumerRegistration{
			ID:     "corpus.documents",
			Events: []EventKey{EventKeyFor[contractTestBody]()},
		},
	}
	if _, err := NewDispatcher(DispatcherDependencies{
		Journal: service, Zones: contractTestZones{}, Consumers: []Consumer{consumer}, Positions: positions,
		ReadLimit: 10, PollInterval: time.Millisecond,
	}); err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	if _, err := NewDispatcher(DispatcherDependencies{
		Journal: service, Zones: contractTestZones{}, Consumers: []Consumer{consumer, consumer}, Positions: positions,
		ReadLimit: 10, PollInterval: time.Millisecond,
	}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("duplicate Consumer error = %v, want ErrInvalidRegistration", err)
	}
	unsupported := contractTestConsumer{registration: ConsumerRegistration{
		ID: "unsupported", Events: []EventKey{EventKeyFor[unregisteredBody]()},
	}}
	if _, err := NewDispatcher(DispatcherDependencies{
		Journal: service, Zones: contractTestZones{}, Consumers: []Consumer{unsupported}, Positions: positions,
		ReadLimit: 10, PollInterval: time.Millisecond,
	}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("unsupported Consumer error = %v, want ErrInvalidRegistration", err)
	}
}

func newContractTestService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(ServiceDependencies{
		Contracts: []EventContractRegistration{
			JSONEventRegistration[contractTestBody](),
			JSONEventRegistration[contractCompletionBody](),
		},
		AtomicBatches: []AtomicBatchContract{{
			MemberEvents:    []EventKey{EventKeyFor[contractTestBody]()},
			CompletionEvent: EventKeyFor[contractCompletionBody](),
		}},
		Reader: contractTestReader{},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func valueEvent(t *testing.T) ProposedEvent {
	t.Helper()
	occurredAt, err := time.Parse(time.RFC3339, testEventTime)
	if err != nil {
		t.Fatal(err)
	}
	return ProposedEvent{
		EventID:       testEventID,
		ZoneID:        testZoneID,
		StreamID:      "test/stream",
		Type:          EventKeyFor[contractTestBody]().Type,
		SchemaVersion: 1,
		OccurredAt:    occurredAt,
		CorrelationID: testCorrelation,
		CausationID:   testCausationID,
		Body:          `{"value":"kept"}`,
	}
}

func completionEvent(t *testing.T) ProposedEvent {
	t.Helper()
	event := valueEvent(t)
	event.EventID = "55555555-5555-4555-8555-555555555555"
	event.Type = EventKeyFor[contractCompletionBody]().Type
	event.Body = `{"count":1}`
	return event
}

type contractTestReader struct{}

func (contractTestReader) ReadEventType(context.Context, ZoneID, EventType, Position, int) ([]Event, error) {
	return nil, nil
}

func (contractTestReader) ReadEntries(context.Context, ZoneID, uint64, int) ([]Event, error) {
	return nil, nil
}

func (contractTestReader) ReadStreamAfter(context.Context, StreamID, StreamSequence, int) ([]Event, error) {
	return nil, nil
}

type contractTestZones struct{}

func (contractTestZones) Resolve(_ context.Context, id ZoneID) (zone.Definition, error) {
	return zone.NewRootDefinition(id, "", time.Now().UTC())
}

type contractTestPositions struct{}

func (*contractTestPositions) Position(context.Context, ConsumerID, EventType) (Position, error) {
	return 0, nil
}
func (*contractTestPositions) Advance(context.Context, ConsumerID, EventType, Position, Position) error {
	return nil
}
func (*contractTestPositions) PendingZone(context.Context, ConsumerID, EventKey) (ZoneID, error) {
	return "", nil
}

type contractTestConsumer struct {
	registration ConsumerRegistration
}

func (c contractTestConsumer) ConsumerRegistration() ConsumerRegistration {
	return c.registration
}

func (contractTestConsumer) Handle(context.Context, Event) error { return nil }
