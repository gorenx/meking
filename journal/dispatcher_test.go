package journal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/memoria-space/meking/zone"
)

type dispatcherBody struct{}

func (dispatcherBody) EventType() string     { return "dispatcher.ready" }
func (dispatcherBody) SchemaVersion() uint32 { return 1 }

type dispatcherReader struct {
	events []Event
}

func (reader dispatcherReader) ReadEventType(_ context.Context, zoneID ZoneID, eventType EventType, from Position, limit int) ([]Event, error) {
	result := make([]Event, 0, limit)
	for _, event := range reader.events {
		if event.ZoneID != zoneID || event.Type != eventType || Position(event.Sequence) < from {
			continue
		}
		result = append(result, event)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (reader dispatcherReader) ReadEntries(context.Context, ZoneID, uint64, int) ([]Event, error) {
	return nil, nil
}

func (reader dispatcherReader) ReadStreamAfter(_ context.Context, streamID StreamID, after StreamSequence, limit int) ([]Event, error) {
	result := make([]Event, 0, limit)
	for _, event := range reader.events {
		if event.StreamID != streamID || event.StreamSequence <= after {
			continue
		}
		result = append(result, event)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

type dispatcherPositions struct {
	mutex     sync.Mutex
	zones     []ZoneID
	positions map[ZoneID]Position
}

func (positions *dispatcherPositions) Position(ctx context.Context, _ ConsumerID, _ EventType) (Position, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return 0, err
	}
	positions.mutex.Lock()
	defer positions.mutex.Unlock()
	return positions.positions[zoneID], nil
}

func (positions *dispatcherPositions) Advance(ctx context.Context, _ ConsumerID, _ EventType, expected Position, position Position) error {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	positions.mutex.Lock()
	defer positions.mutex.Unlock()
	if positions.positions == nil {
		positions.positions = make(map[ZoneID]Position)
	}
	if positions.positions[zoneID] != expected {
		return ErrConsumerPosition
	}
	positions.positions[zoneID] = position
	return nil
}

func (positions *dispatcherPositions) PendingZone(context.Context, ConsumerID, EventKey) (ZoneID, error) {
	positions.mutex.Lock()
	defer positions.mutex.Unlock()
	if len(positions.zones) == 0 {
		return "", nil
	}
	zoneID := positions.zones[0]
	positions.zones = positions.zones[1:]
	return zoneID, nil
}

type dispatcherZones struct{}

func (dispatcherZones) Resolve(_ context.Context, id zone.ID) (zone.Definition, error) {
	return zone.NewRootDefinition(id, "", time.Now().UTC())
}

type dispatcherConsumer struct {
	mutex   sync.Mutex
	handled []ZoneID
	failure error
}

func (*dispatcherConsumer) ConsumerRegistration() ConsumerRegistration {
	return ConsumerRegistration{ID: "dispatcher.test", Events: []EventKey{EventKeyFor[dispatcherBody]()}}
}

func (consumer *dispatcherConsumer) Handle(ctx context.Context, _ Event) error {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	consumer.mutex.Lock()
	consumer.handled = append(consumer.handled, zoneID)
	consumer.mutex.Unlock()
	return consumer.failure
}

func TestDispatcherProcessesOnePendingZonePerSchedulingStep(t *testing.T) {
	firstZone := ZoneID("10000000-0000-4000-8000-000000000001")
	secondZone := ZoneID("20000000-0000-4000-8000-000000000002")
	key := EventKeyFor[dispatcherBody]()
	reader := dispatcherReader{events: []Event{
		dispatcherEvent(firstZone, 0, "event-1", key),
		dispatcherEvent(secondZone, 0, "event-2", key),
	}}
	service, err := NewService(ServiceDependencies{
		Contracts: []EventContractRegistration{JSONEventRegistration[dispatcherBody]()},
		Reader:    reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	positions := &dispatcherPositions{zones: []ZoneID{firstZone, secondZone}, positions: make(map[ZoneID]Position)}
	consumer := &dispatcherConsumer{}
	dispatcher, err := NewDispatcher(DispatcherDependencies{
		Journal: service, Zones: dispatcherZones{}, Consumers: []Consumer{consumer}, Positions: positions,
		ReadLimit: 10, PollInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	registered := dispatcher.consumers[0]
	processed, err := dispatcher.processPending(t.Context(), registered, key)
	if err != nil || !processed {
		t.Fatalf("first processPending() = %t, %v", processed, err)
	}
	if len(consumer.handled) != 1 || consumer.handled[0] != firstZone {
		t.Fatalf("first scheduling step handled Zones = %v", consumer.handled)
	}
	processed, err = dispatcher.processPending(t.Context(), registered, key)
	if err != nil || !processed {
		t.Fatalf("second processPending() = %t, %v", processed, err)
	}
	if len(consumer.handled) != 2 || consumer.handled[1] != secondZone {
		t.Fatalf("second scheduling step handled Zones = %v", consumer.handled)
	}
}

func TestDispatcherLeavesFailedEventPositionUnchanged(t *testing.T) {
	zoneID := ZoneID("10000000-0000-4000-8000-000000000001")
	key := EventKeyFor[dispatcherBody]()
	service, err := NewService(ServiceDependencies{
		Contracts: []EventContractRegistration{JSONEventRegistration[dispatcherBody]()},
		Reader:    dispatcherReader{events: []Event{dispatcherEvent(zoneID, 0, "event-1", key)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("consumer failed")
	positions := &dispatcherPositions{zones: []ZoneID{zoneID}, positions: make(map[ZoneID]Position)}
	consumer := &dispatcherConsumer{failure: failure}
	dispatcher, err := NewDispatcher(DispatcherDependencies{
		Journal: service, Zones: dispatcherZones{}, Consumers: []Consumer{consumer}, Positions: positions,
		ReadLimit: 10, PollInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = dispatcher.processPending(t.Context(), dispatcher.consumers[0], key)
	if !errors.Is(err, failure) {
		t.Fatalf("processPending() error = %v", err)
	}
	if positions.positions[zoneID] != 0 {
		t.Fatalf("Position after failure = %d", positions.positions[zoneID])
	}
}

func dispatcherEvent(zoneID ZoneID, sequence EventSequence, eventID EventID, key EventKey) Event {
	return Event{
		ProposedEvent: ProposedEvent{
			EventID: eventID, ZoneID: zoneID, StreamID: StreamID("stream/" + string(zoneID)),
			Type: key.Type, SchemaVersion: key.Version, OccurredAt: time.Now().UTC(), Body: `{}`,
		},
		Sequence: sequence, StreamSequence: 1,
	}
}
