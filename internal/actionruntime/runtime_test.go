package actionruntime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/zone"
)

const runtimeZoneID zone.ID = "10000000-0000-4000-8000-000000000001"

type runtimeBody struct{}

func (runtimeBody) EventType() string     { return "runtime.ready" }
func (runtimeBody) SchemaVersion() uint32 { return 1 }

type runtimeReader struct{ events []journal.Event }

func (reader runtimeReader) ReadEventType(_ context.Context, zoneID journal.ZoneID, eventType journal.EventType, from journal.Position, limit int) ([]journal.Event, error) {
	result := make([]journal.Event, 0, limit)
	for _, event := range reader.events {
		if event.ZoneID != zoneID || event.Type != eventType || journal.Position(event.Sequence) < from {
			continue
		}
		result = append(result, event)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (reader runtimeReader) ReadEntries(context.Context, journal.ZoneID, uint64, int) ([]journal.Event, error) {
	return nil, nil
}

func (reader runtimeReader) ReadStreamAfter(context.Context, journal.StreamID, journal.StreamSequence, int) ([]journal.Event, error) {
	return nil, nil
}

type runtimePositions struct {
	mutex    sync.Mutex
	position journal.Position
}

func (positions *runtimePositions) Position(context.Context, journal.ConsumerID, journal.EventType) (journal.Position, error) {
	positions.mutex.Lock()
	defer positions.mutex.Unlock()
	return positions.position, nil
}

func (positions *runtimePositions) Advance(_ context.Context, _ journal.ConsumerID, _ journal.EventType, expected journal.Position, position journal.Position) error {
	positions.mutex.Lock()
	defer positions.mutex.Unlock()
	if positions.position == expected {
		positions.position = position
	}
	return nil
}

func (*runtimePositions) PendingZone(context.Context, journal.ConsumerID, journal.EventKey) (journal.ZoneID, error) {
	return runtimeZoneID, nil
}

func TestRuntimeProcessesOnlyRequestedZone(t *testing.T) {
	key := journal.EventKeyFor[runtimeBody]()
	event := journal.Event{
		ProposedEvent: journal.ProposedEvent{
			EventID: "event-1", ZoneID: runtimeZoneID, StreamID: "runtime/1",
			Type: key.Type, SchemaVersion: key.Version, OccurredAt: time.Now().UTC(), Body: `{}`,
		},
		Sequence: 0, StreamSequence: 1,
	}
	service, err := journal.NewService(journal.ServiceDependencies{
		Contracts: []journal.EventContractRegistration{journal.JSONEventRegistration[runtimeBody]()},
		Reader:    runtimeReader{events: []journal.Event{event}},
	})
	if err != nil {
		t.Fatal(err)
	}
	positions := &runtimePositions{}
	handled := make(chan zone.ID, 1)
	runtime, err := New(Dependencies{
		Name: "Runtime test", ConsumerID: "runtime.test", Journal: service,
		Positions: positions, EventKeys: []journal.EventKey{key}, ReadLimit: 10,
		Handle: func(ctx context.Context, _ journal.Event) error {
			zoneID, err := zone.RequireID(ctx)
			if err == nil {
				handled <- zoneID
			}
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	zoneContext, err := zone.NewContext(ctx, runtimeZoneID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Retry(zoneContext); err != nil {
		t.Fatal(err)
	}
	select {
	case zoneID := <-handled:
		if zoneID != runtimeZoneID {
			t.Fatalf("handled Zone = %q", zoneID)
		}
	case <-time.After(time.Second):
		t.Fatal("requested Zone was not processed")
	}
	cancel()
	<-done
}
