package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/memoria-space/meking/corpus/message"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

func TestMessageLogAppendsInSessionOrderAndReusesTextUnit(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	log := newTestMessageLog(t, database, store)
	ctx := testZoneContext(t)
	first := newTestMessage(t, "message-1", "user", "same text")
	second := newTestMessage(t, "message-2", "assistant", "same text")

	occurrences, err := log.Append(ctx, []message.Message{first, second})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if occurrences[0].Position != 0 || occurrences[1].Position != 1 {
		t.Fatalf("Message positions = %d, %d", occurrences[0].Position, occurrences[1].Position)
	}
	if occurrences[0].Message.TextUnit.ID != occurrences[1].Message.TextUnit.ID {
		t.Fatalf("TextUnit IDs differ: %q != %q", occurrences[0].Message.TextUnit.ID, occurrences[1].Message.TextUnit.ID)
	}
	var textUnitCount int
	if err := database.QueryRow(`SELECT count(*) FROM text_units`).Scan(&textUnitCount); err != nil {
		t.Fatal(err)
	}
	if textUnitCount != 1 {
		t.Fatalf("TextUnit count = %d, want 1", textUnitCount)
	}

	reused, err := log.Append(ctx, []message.Message{first, second})
	if err != nil {
		t.Fatalf("duplicate Append() error = %v", err)
	}
	if reused[0] != occurrences[0] || reused[1] != occurrences[1] {
		t.Fatalf("reused Messages = %#v, want %#v", reused, occurrences)
	}
}

func TestMessageLogScopesPositionBySessionZone(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	log := newTestMessageLog(t, database, store)
	value := newTestMessage(t, "message-1", "user", "text")
	for _, ctx := range []context.Context{
		testZoneContextWithID(t, "10000000-0000-4000-8000-000000000001"),
		testZoneContextWithID(t, "20000000-0000-4000-8000-000000000002"),
	} {
		occurrences, err := log.Append(ctx, []message.Message{value})
		if err != nil {
			t.Fatal(err)
		}
		if occurrences[0].Position != 0 {
			t.Fatalf("first Session position = %d, want 0", occurrences[0].Position)
		}
	}
}

func TestMessageLogRejectsIdentityAndSequenceConflicts(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	log := newTestMessageLog(t, database, store)
	ctx := testZoneContext(t)
	first := newTestMessage(t, "message-1", "user", "first")
	if _, err := log.Append(ctx, []message.Message{first}); err != nil {
		t.Fatal(err)
	}
	changed := newTestMessage(t, "message-1", "user", "changed")
	if _, err := log.Append(ctx, []message.Message{changed}); !errors.Is(err, message.ErrIdentityConflict) {
		t.Fatalf("identity conflict error = %v", err)
	}
	second := newTestMessage(t, "message-2", "assistant", "second")
	occurrences, err := log.Append(ctx, []message.Message{second, first})
	if err != nil {
		t.Fatalf("mixed existing and new Append() error = %v", err)
	}
	if occurrences[0].Message != second || occurrences[0].Position != 1 ||
		occurrences[1].Message != first || occurrences[1].Position != 0 {
		t.Fatalf("mixed Message positions = %#v", occurrences)
	}
}

func TestMessageLogReadsInSessionOrder(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	log := newTestMessageLog(t, database, store)
	ctx := testZoneContext(t)
	first := newTestMessage(t, "message-1", "user", "first")
	second := newTestMessage(t, "message-2", "assistant", "second")
	if _, err := log.Append(ctx, []message.Message{first, second}); err != nil {
		t.Fatal(err)
	}
	read, err := log.Read(ctx, []string{second.ID, first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if read[0].Message.ID != first.ID || read[1].Message.ID != second.ID {
		t.Fatalf("Read() order = %q, %q", read[0].Message.ID, read[1].Message.ID)
	}
	if _, err := log.Read(ctx, []string{"missing"}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("missing Message error = %v", err)
	}
}

func newTestMessage(t *testing.T, id, role, text string) message.Message {
	t.Helper()
	value, err := message.New(id, role, text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func newTestMessageLog(t *testing.T, database *sql.DB, entries message.Entries) *message.Log {
	t.Helper()
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	log, err := message.NewLog(message.Dependencies{
		Transaction: transactions,
		Entries:     entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	return log
}
