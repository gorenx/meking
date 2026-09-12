package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/journal"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

func TestStoreReadsEventTypeSequenceAndBrowseOffset(t *testing.T) {
	database := openJournalTestDatabaseNamed(t, "journal-read")
	store, service, publisher, transactions := openJournalComponents(t, database)
	events := []journal.ProposedEvent{
		appendEventForStream("event-one", "stream-one", `{"value":"one"}`),
		appendEventForStream("event-two", "stream-two", `{"value":"two"}`),
		appendEventForStream("event-three", "stream-one", `{"value":"three"}`),
	}
	ctx := scopedJournalContext(t)
	if err := transactions.WithTx(ctx, func(ctx context.Context) error {
		return publisher.Publish(ctx, events)
	}); err != nil {
		t.Fatal(err)
	}

	typed, err := service.Events(ctx, journal.EventKeyFor[appendTestBody]().Type, 1, 3, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(typed) != 2 || typed[0].EventID != "event-two" || typed[0].Sequence != 1 || typed[1].Sequence != 2 {
		t.Fatalf("EventType events = %#v", typed)
	}
	entries, err := service.Entries(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].EventID != "event-two" || entries[1].EventID != "event-three" {
		t.Fatalf("Journal entries = %#v", entries)
	}
	stream, err := service.StreamEventsAfter(ctx, "stream-one", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stream) != 2 || stream[0].StreamSequence != 1 || stream[1].StreamSequence != 2 {
		t.Fatalf("Stream events = %#v", stream)
	}
	if _, err := store.ReadEntries(ctx, "11111111-1111-4111-8111-111111111111", 0, 0); !errors.Is(err, journal.ErrInvalidEvent) {
		t.Fatalf("ReadEntries(invalid limit) error = %v", err)
	}
}

func TestStoreSummarizesPendingExactZoneAndEventKey(t *testing.T) {
	database := openJournalTestDatabaseNamed(t, "journal-pending")
	_, service, publisher, transactions := openJournalComponents(t, database)
	firstOccurredAt := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	first := appendEventForStream("event-one", "stream-one", `{"value":"one"}`)
	first.OccurredAt = firstOccurredAt
	second := appendEventForStream("event-two", "stream-two", `{"value":"two"}`)
	second.OccurredAt = firstOccurredAt.Add(time.Minute)
	ctx := scopedJournalContext(t)
	if err := transactions.WithTx(ctx, func(ctx context.Context) error {
		return publisher.Publish(ctx, []journal.ProposedEvent{first, second})
	}); err != nil {
		t.Fatal(err)
	}
	otherZone, err := zone.NewContext(t.Context(), "20000000-0000-4000-8000-000000000002")
	if err != nil {
		t.Fatal(err)
	}
	third := appendEventForStream("event-three", "stream-three", `{"value":"three"}`)
	if err := transactions.WithTx(otherZone, func(ctx context.Context) error {
		return publisher.Publish(ctx, []journal.ProposedEvent{third})
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := service.PendingEvents(ctx, journal.EventKeyFor[appendTestBody](), 1)
	if err != nil {
		t.Fatal(err)
	}
	if pending.ThroughPosition != 2 || pending.Count != 1 || !pending.Since.Equal(second.OccurredAt) {
		t.Fatalf("PendingEvents = %#v", pending)
	}
}

func TestStoreAdvancesConsumerPositionPerZoneAndEventType(t *testing.T) {
	database := openJournalTestDatabaseNamed(t, "journal-positions")
	store, _, publisher, transactions := openJournalComponents(t, database)
	ctx := scopedJournalContext(t)
	if err := transactions.WithTx(ctx, func(ctx context.Context) error {
		return publisher.Publish(ctx, []journal.ProposedEvent{
			appendEventForStream("event-one", "stream-one", `{"value":"one"}`),
			appendEventForStream("event-two", "stream-one", `{"value":"two"}`),
		})
	}); err != nil {
		t.Fatal(err)
	}
	eventType := journal.EventKeyFor[appendTestBody]().Type
	position, err := store.Position(ctx, "consumer-one", eventType)
	if err != nil || position != 0 {
		t.Fatalf("new Consumer Position = %d, %v", position, err)
	}
	if err := store.Advance(ctx, "consumer-one", eventType, 0, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.Advance(ctx, "consumer-one", eventType, 0, 1); !errors.Is(err, journal.ErrConsumerPosition) {
		t.Fatalf("stale Advance() error = %v", err)
	}
	position, err = store.Position(ctx, "consumer-one", eventType)
	if err != nil || position != 2 {
		t.Fatalf("stored Consumer Position = %d, %v", position, err)
	}
}

func openJournalComponents(t *testing.T, database *sql.DB) (*Store, *journal.Service, journal.Publisher, *transactionsqlite.Tx) {
	t.Helper()
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database, transactions)
	if err != nil {
		t.Fatal(err)
	}
	service, err := journal.NewService(journal.ServiceDependencies{
		Contracts: []journal.EventContractRegistration{journal.JSONEventRegistration[appendTestBody]()},
		Reader:    store,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := journal.NewPublisher(service, store)
	if err != nil {
		t.Fatal(err)
	}
	return store, service, publisher, transactions
}

func openJournalTestDatabaseNamed(t *testing.T, name string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if err := database.Ping(); err != nil {
		t.Fatal(err)
	}
	return database
}

func appendEventForStream(id journal.EventID, stream journal.StreamID, body string) journal.ProposedEvent {
	return journal.ProposedEvent{
		EventID: id, StreamID: stream, Type: journal.EventKeyFor[appendTestBody]().Type,
		SchemaVersion: 1, OccurredAt: time.Date(2026, 7, 31, 19, 0, 0, 0, time.UTC),
		CorrelationID: "request-one", Body: body,
	}
}
