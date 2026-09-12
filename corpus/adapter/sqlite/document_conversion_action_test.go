package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	corpusjournal "github.com/memoria-space/meking/corpus/adapter/journal"
	"github.com/memoria-space/meking/corpus/document"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
	journalsqlite "github.com/memoria-space/meking/journal/adapter/sqlite"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

const documentConversionTestZoneID = "10000000-0000-4000-8000-000000000001"

func TestDocumentConversionActionPreparesText(t *testing.T) {
	fixture := openDocumentConversionFixture(t)
	fixture.advance(t)

	events, err := fixture.journal.Entries(testZoneContext(t), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("Journal events = %d, want 2", len(events))
	}
	body, err := journalcore.DecodeJSONBody[corpusevents.TextPreparedV1](events[1])
	if err != nil {
		t.Fatal(err)
	}
	if body.DocumentID != corpusevents.DocumentID(fixture.documentID) || body.TextID == "" {
		t.Fatalf("TextPrepared body = %#v", body)
	}
	var textBody string
	if err := fixture.database.QueryRow(
		`SELECT body FROM texts WHERE id = ?`,
		body.TextID,
	).Scan(&textBody); err != nil {
		t.Fatal(err)
	}
	if textBody != "first\nsecond" {
		t.Fatalf("prepared Text body = %q", textBody)
	}
}

func TestDocumentConversionActionPreservesSourceEventIdentity(t *testing.T) {
	fixture := openDocumentConversionFixture(t)
	fixture.advance(t)

	events, err := fixture.journal.Entries(testZoneContext(t), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("Journal events = %d, want 2", len(events))
	}
	if events[1].CausationID != events[0].EventID ||
		events[1].CorrelationID != events[0].CorrelationID {
		t.Fatalf("TextPrepared envelope = %#v", events[1])
	}
}

func TestDocumentConversionActionAdvancesJournalConsumerPosition(t *testing.T) {
	fixture := openDocumentConversionFixture(t)
	fixture.advance(t)

	var cursor int64
	if err := fixture.database.QueryRow(`
		SELECT position
		FROM journal_consumer_positions
		WHERE zone_id = ? AND consumer_id = ? AND event_type = ?
	`, documentConversionTestZoneID, "corpus.document-conversion", "corpus.document_recorded").Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if cursor != 1 {
		t.Fatalf("Consumer Position = %d, want 1", cursor)
	}
}

func TestDocumentConversionActionDoesNotRepeatCompletedDocument(t *testing.T) {
	fixture := openDocumentConversionFixture(t)
	fixture.advance(t)
	if err := fixture.action.Retry(testZoneContext(t)); !errors.Is(err, actionruntime.ErrNoPendingEvents) {
		t.Fatalf("Retry() error = %v, want ErrNoPendingEvents", err)
	}

	events, err := fixture.journal.Entries(testZoneContext(t), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("Journal events after repeated advance = %d, want 2", len(events))
	}
}

type documentConversionFixture struct {
	database   *sql.DB
	journal    *journalcore.Service
	action     *corpusjournal.DocumentConversionAction
	documentID document.ID
}

func openDocumentConversionFixture(t *testing.T) documentConversionFixture {
	t.Helper()
	database := openCorpusDatabase(t)
	service, journalService, _ := openDocumentService(t, database, nil)
	receipt, err := service.SubmitDocument(testZoneContext(t), document.UploadCommand{
		Name:      "source.txt",
		MediaType: "text/plain",
		Content:   bytes.NewBufferString("  first\r\nsecond  "),
	})
	if err != nil {
		t.Fatal(err)
	}
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	journalStore, err := journalsqlite.NewStore(database, transactions)
	if err != nil {
		t.Fatal(err)
	}
	action, err := corpusjournal.NewDocumentConversionAction(
		corpusjournal.DocumentConversionActionDependencies{
			Journal:   journalService,
			Positions: journalStore,
			Texts:     service,
			Merger:    noOpCorpusZoneMerger{},
			ReadLimit: 16,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return documentConversionFixture{
		database:   database,
		journal:    journalService,
		action:     action,
		documentID: receipt.DocumentID,
	}
}

func (fixture documentConversionFixture) advance(t *testing.T) {
	t.Helper()
	runContext, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- fixture.action.Run(runContext)
	}()
	if err := fixture.action.Retry(testZoneContext(t)); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		var cursor uint64
		if err := fixture.database.QueryRow(`
			SELECT position
			FROM journal_consumer_positions
			WHERE zone_id = ? AND consumer_id = ? AND event_type = ?
		`, documentConversionTestZoneID, "corpus.document-conversion", "corpus.document_recorded").Scan(&cursor); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatal(err)
			}
		}
		if cursor == 1 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("Run() stopped before Consumer Position reached 1: %v", err)
		case <-deadline.C:
			t.Fatal("Consumer Position did not reach 1")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
}

type noOpCorpusZoneMerger struct{}

func (noOpCorpusZoneMerger) MergeChildDocuments(context.Context) error { return nil }
func (noOpCorpusZoneMerger) MergeChildTexts(context.Context) error     { return nil }
func (noOpCorpusZoneMerger) MergeTextUnits(context.Context) error      { return nil }
