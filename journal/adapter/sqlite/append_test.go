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
	_ "modernc.org/sqlite"
)

type appendTestBody struct {
	Value string `json:"value"`
}

func (appendTestBody) EventType() string     { return "test.appended" }
func (appendTestBody) SchemaVersion() uint32 { return 1 }

func TestPublisherAppendsThroughOwnerTransaction(t *testing.T) {
	database := openJournalTestDatabase(t)
	if _, err := database.Exec(`CREATE TABLE owner_results (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database, transactions)
	if err != nil {
		t.Fatal(err)
	}
	service, err := journal.NewService(journal.ServiceDependencies{
		Contracts: []journal.EventContractRegistration{
			journal.JSONEventRegistration[appendTestBody](),
		},
		Reader: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := journal.NewPublisher(service, store)
	if err != nil {
		t.Fatal(err)
	}
	events := []journal.ProposedEvent{
		appendEvent("event-one", `{"value":"one"}`),
		appendEvent("event-two", `{"value":"two"}`),
	}
	if err := transactions.WithTx(scopedJournalContext(t), func(ctx context.Context) error {
		executor, err := transactionsqlite.Current(ctx, database)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO owner_results (id) VALUES ('result-one')`); err != nil {
			return err
		}
		return publisher.Publish(ctx, events)
	}); err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}

	assertJournalCounts(t, database, 1, 2)
	rows, err := database.Query(`SELECT event_sequence, stream_sequence, body FROM journal_events ORDER BY event_sequence`)
	if err != nil {
		t.Fatal(err)
	}
	for index, expectedBody := range []string{`{"value":"one"}`, `{"value":"two"}`} {
		if !rows.Next() {
			t.Fatalf("missing Journal row %d", index)
		}
		var eventSequence, streamSequence int64
		var body string
		if err := rows.Scan(&eventSequence, &streamSequence, &body); err != nil {
			t.Fatal(err)
		}
		if eventSequence != int64(index) || streamSequence != int64(index+1) || body != expectedBody {
			t.Fatalf("row %d = EventType Sequence %d Stream Sequence %d body %q", index, eventSequence, streamSequence, body)
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	if err := transactions.WithTx(scopedJournalContext(t), func(ctx context.Context) error {
		return publisher.Publish(ctx, events)
	}); err != nil {
		t.Fatalf("idempotent Publish() error = %v", err)
	}
	assertJournalCounts(t, database, 1, 2)
}

func TestPublisherFailureRollsBackOwnerResult(t *testing.T) {
	database := openJournalTestDatabase(t)
	if _, err := database.Exec(`CREATE TABLE owner_results (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database, transactions)
	if err != nil {
		t.Fatal(err)
	}
	service, err := journal.NewService(journal.ServiceDependencies{
		Contracts: []journal.EventContractRegistration{
			journal.JSONEventRegistration[appendTestBody](),
		},
		Reader: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := journal.NewPublisher(service, store)
	if err != nil {
		t.Fatal(err)
	}
	original := appendEvent("event-one", `{"value":"one"}`)
	if err := transactions.WithTx(scopedJournalContext(t), func(ctx context.Context) error {
		return publisher.Publish(ctx, []journal.ProposedEvent{original})
	}); err != nil {
		t.Fatal(err)
	}
	conflict := original
	conflict.Body = `{"value":"different"}`
	err = transactions.WithTx(scopedJournalContext(t), func(ctx context.Context) error {
		executor, err := transactionsqlite.Current(ctx, database)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO owner_results (id) VALUES ('must-roll-back')`); err != nil {
			return err
		}
		return publisher.Publish(ctx, []journal.ProposedEvent{conflict})
	})
	if !errors.Is(err, journal.ErrEventConflict) {
		t.Fatalf("conflicting Publish() error = %v, want ErrEventConflict", err)
	}
	assertJournalCounts(t, database, 0, 1)
	if err := publisher.Publish(scopedJournalContext(t), []journal.ProposedEvent{original}); !errors.Is(err, transactionsqlite.ErrNoTransaction) {
		t.Fatalf("Publish() outside transaction error = %v, want ErrNoTransaction", err)
	}
}

func appendEvent(id journal.EventID, body string) journal.ProposedEvent {
	return journal.ProposedEvent{
		EventID:       id,
		StreamID:      "stream-one",
		Type:          journal.EventKeyFor[appendTestBody]().Type,
		SchemaVersion: 1,
		OccurredAt:    time.Date(2026, 7, 31, 19, 0, 0, 0, time.UTC),
		CorrelationID: "request-one",
		Body:          body,
	}
}

func scopedJournalContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := zone.NewContext(t.Context(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func assertJournalCounts(t *testing.T, database *sql.DB, ownerCount, eventCount int) {
	t.Helper()
	var storedOwnerCount int
	if err := database.QueryRow(`SELECT count(*) FROM owner_results`).Scan(&storedOwnerCount); err != nil {
		t.Fatal(err)
	}
	var storedEventCount int
	if err := database.QueryRow(`SELECT count(*) FROM journal_events`).Scan(&storedEventCount); err != nil {
		t.Fatal(err)
	}
	if storedOwnerCount != ownerCount || storedEventCount != eventCount {
		t.Fatalf("stored counts = owner %d events %d, want owner %d events %d", storedOwnerCount, storedEventCount, ownerCount, eventCount)
	}
}

func openJournalTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:journal-append?mode=memory&cache=shared")
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
