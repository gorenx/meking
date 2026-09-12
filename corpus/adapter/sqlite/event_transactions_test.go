package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/adapter/local"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/textunits"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

func TestCorpusApplicationsCommitOwnerResultsOnlyWithTheirEvents(t *testing.T) {
	database := openCorpusDatabase(t)
	producer := &transactionCorpusProducer{}
	documents, _, _ := openDocumentService(t, database, producer)
	receipt, err := documents.SubmitDocument(testZoneContext(t), document.UploadCommand{
		Name: "source.txt", MediaType: "text/plain",
		Content: bytes.NewBufferString("knowledge graph"),
	})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("event publication failed")
	producer.failure = failure
	preparation := corpus.TextPreparation{
		EventID: producer.events[0].EventID, CorrelationID: producer.events[0].CorrelationID,
		OccurredAt: time.Date(2026, time.July, 31, 20, 1, 0, 0, time.UTC),
		DocumentID: receipt.DocumentID,
	}
	if _, err := documents.PrepareText(testZoneContext(t), preparation); !errors.Is(err, failure) {
		t.Fatalf("PrepareText() error = %v, want publish failure", err)
	}
	assertCorpusTableCount(t, database, "texts", 0)

	producer.failure = nil
	prepared, err := documents.PrepareText(testZoneContext(t), preparation)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	content, err := local.NewContentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	documentService, err := document.NewManager(store.DocumentRepository(), content)
	if err != nil {
		t.Fatal(err)
	}
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	chunking := textunits.Chunking{
		Type: textunits.TokenChunking, Size: 32,
		EncodingModel: textunits.DefaultEncodingModel,
	}
	sets, err := corpus.NewService(
		store,
		corpus.WithDocument(documentService),
		corpus.WithEventPublishing(transactions, producer),
		corpus.WithChunking(chunking),
		corpus.WithLogger(slog.New(slog.DiscardHandler)),
	)
	if err != nil {
		t.Fatal(err)
	}
	textUnitPreparation := corpus.PrepareTextUnitsInput{
		EventID: "text-prepared", CorrelationID: "request-one",
		OccurredAt: time.Date(2026, time.July, 31, 20, 2, 0, 0, time.UTC),
		TextID:     prepared.ID,
	}
	producer.failure = failure
	if _, err := sets.PrepareTextUnits(testZoneContext(t), textUnitPreparation); !errors.Is(err, failure) {
		t.Fatalf("PrepareTextUnits() error = %v, want publish failure", err)
	}
	assertCorpusTableCount(t, database, "text_chunking_progress", 1)
	assertCorpusTableCount(t, database, "text_unit_spans", 0)
	assertCorpusTableCount(t, database, "text_units", 0)

	producer.failure = nil
	corporaID, err := sets.PrepareTextUnits(testZoneContext(t), textUnitPreparation)
	if err != nil {
		t.Fatal(err)
	}
	if err := corpus.ValidateCorporaID(corporaID); err != nil {
		t.Fatalf("PrepareTextUnits() CorporaID = %q: %v", corporaID, err)
	}
	assertCorpusTableCount(t, database, "text_unit_spans", 1)
	assertCorpusTableCount(t, database, "text_units", 1)
	if len(producer.events) != 3 {
		t.Fatalf("published Corpus events = %d, want 3", len(producer.events))
	}
}

type transactionCorpusProducer struct {
	events  []corpus.Event
	failure error
}

func (producer *transactionCorpusProducer) Publish(
	_ context.Context,
	events []corpus.Event,
) error {
	if producer.failure != nil {
		return producer.failure
	}
	producer.events = append(producer.events, events...)
	return nil
}

func assertCorpusTableCount(t *testing.T, database interface {
	QueryRow(string, ...any) *sql.Row
}, table string, want int) {
	t.Helper()
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}
