package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"

	"github.com/memoria-space/meking/corpus"
	corpusjournal "github.com/memoria-space/meking/corpus/adapter/journal"
	"github.com/memoria-space/meking/corpus/adapter/local"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
	journalcore "github.com/memoria-space/meking/journal"
	journalsqlite "github.com/memoria-space/meking/journal/adapter/sqlite"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

func TestSubmitDocumentRecordsZoneFact(t *testing.T) {
	database := openCorpusDatabase(t)
	service, journalService, _ := openDocumentService(t, database, nil)
	zoneA := testZoneContextWithID(t, "10000000-0000-4000-8000-000000000001")
	zoneB := testZoneContextWithID(t, "20000000-0000-4000-8000-000000000002")

	first, err := service.SubmitDocument(zoneA, document.UploadCommand{
		Name: "source.txt", MediaType: "text/plain",
		Content: bytes.NewBufferString("knowledge"),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.SubmitDocument(zoneB, document.UploadCommand{
		Name: "renamed.md", MediaType: "text/markdown",
		Content: bytes.NewBufferString("knowledge"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.DocumentID == "" || first.ContentDigest == "" {
		t.Fatalf("upload receipts = %#v and %#v", first, second)
	}
	if events, err := journalService.Entries(zoneA, 0, 10); err != nil || len(events) != 1 {
		t.Fatalf("upload Journal events = %#v, %v", events, err)
	}
	assertDocumentCounts(t, database, 1, 2, 2)

	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{zoneA, zoneB} {
		if _, err := store.DocumentRepository().Get(ctx, first.DocumentID); err != nil {
			t.Fatalf("read submitted Document: %v", err)
		}
	}
}

func TestSubmitDocumentRollsBackWhenFactPublicationFails(t *testing.T) {
	database := openCorpusDatabase(t)
	failure := errors.New("publish failed")
	service, _, content := openDocumentService(t, database, failingCorpusProducer{failure: failure})
	ctx := testZoneContext(t)
	_, err := service.SubmitDocument(ctx, document.UploadCommand{
		Name: "source.txt", MediaType: "text/plain",
		Content: bytes.NewBufferString("knowledge"),
	})
	if !errors.Is(err, failure) {
		t.Fatalf("SubmitDocument() error = %v, want publish failure", err)
	}
	assertDocumentCounts(t, database, 0, 0, 0)
	source, err := document.New("source.txt", "text/plain", []byte("knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	location := document.Location(source.Digest[:2] + "/" + source.Digest + ".txt")
	if _, err := content.Open(ctx, location); !errors.Is(err, document.ErrNotFound) {
		t.Fatalf("open content after failed submission error = %v", err)
	}
}

func TestSubmitDocumentRejectsUnsupportedAndOversizedSourcesBeforeStorage(t *testing.T) {
	database := openCorpusDatabase(t)
	plain, _, _ := openDocumentService(t, database, nil)
	upload := document.UploadCommand{
		Name: "source.pdf", MediaType: "application/pdf",
		Content: bytes.NewBufferString("12345"),
	}
	if _, err := plain.SubmitDocument(testZoneContext(t), upload); !errors.Is(err, text.ErrUnsupported) {
		t.Fatalf("plain SubmitDocument() error = %v, want ErrUnsupported", err)
	}
	assertDocumentCounts(t, database, 0, 0, 0)

	richResolver := text.ExtractorResolverFunc(func(name, mediaType string) (text.Extractor, int64, error) {
		if name != "source.pdf" || mediaType != "application/pdf" {
			return nil, 0, text.ErrUnsupported
		}
		return text.PlainTextExtractor{}, 4, nil
	})
	rich, _, _ := openDocumentService(
		t, database, nil, corpus.WithRichDocumentAnalysis(richResolver),
	)
	upload.Content = bytes.NewBufferString("12345")
	if _, err := rich.SubmitDocument(testZoneContext(t), upload); !errors.Is(err, document.ErrContentTooLarge) {
		t.Fatalf("rich SubmitDocument() error = %v, want ErrContentTooLarge", err)
	}
	assertDocumentCounts(t, database, 0, 0, 0)
}

func TestRegisterDocumentOnlyRecordsProjectDocument(t *testing.T) {
	database := openCorpusDatabase(t)
	service, journalService, content := openDocumentService(t, database, nil)
	source, err := document.New("manual.txt", "text/plain", []byte("knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	location := stageDocumentContent(t, content, source, []byte("knowledge"))
	registration := corpus.DocumentRegistration{
		Name: "manual.txt", MediaType: "text/plain", Location: location,
	}
	first, err := service.RegisterDocument(testZoneContext(t), registration)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RegisterDocument(testZoneContext(t), registration)
	if err != nil {
		t.Fatal(err)
	}
	if !first.DocumentCreated || second.DocumentCreated || first.DocumentID != second.DocumentID {
		t.Fatalf("registrations = %#v and %#v", first, second)
	}
	if events, err := journalService.Entries(testZoneContext(t), 0, 10); err != nil || len(events) != 0 {
		t.Fatalf("registration Journal events = %#v, %v", events, err)
	}
	assertDocumentCounts(t, database, 1, 0, 0)
}

func uploadDocument(
	t *testing.T,
	service *corpus.Service,
	ctx context.Context,
	name string,
	content string,
) corpus.DocumentReceipt {
	t.Helper()
	receipt, err := service.SubmitDocument(ctx, document.UploadCommand{
		Name: name, MediaType: "text/plain", Content: bytes.NewBufferString(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func openDocumentService(
	t *testing.T,
	database *sql.DB,
	producer corpus.Producer,
	additionalOptions ...corpus.ServiceOption,
) (*corpus.Service, *journalcore.Service, *local.ContentStore) {
	t.Helper()
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	journalStore, err := journalsqlite.NewStore(database, transactions)
	if err != nil {
		t.Fatal(err)
	}
	journalService, err := journalcore.NewService(journalcore.ServiceDependencies{
		Contracts: corpusjournal.EventContracts(), Reader: journalStore,
	})
	if err != nil {
		t.Fatal(err)
	}
	if producer == nil {
		publisher, err := journalcore.NewPublisher(journalService, journalStore)
		if err != nil {
			t.Fatal(err)
		}
		producer, err = corpusjournal.NewProducer(publisher)
		if err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	content, err := local.NewContentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	documents, err := document.NewManager(store.DocumentRepository(), content)
	if err != nil {
		t.Fatal(err)
	}
	options := []corpus.ServiceOption{
		corpus.WithDocument(documents),
		corpus.WithDocumentCatalog(mustDocumentCatalog(t, store)),
		corpus.WithChunking(textunits.Chunking{
			Type: textunits.TokenChunking, Size: 32,
			EncodingModel: textunits.DefaultEncodingModel,
		}),
		corpus.WithEventPublishing(transactions, producer),
		corpus.WithLogger(slog.New(slog.DiscardHandler)),
	}
	options = append(options, additionalOptions...)
	service, err := corpus.NewService(store, options...)
	if err != nil {
		t.Fatal(err)
	}
	return service, journalService, content
}

func mustDocumentCatalog(t *testing.T, store *Store) *DocumentCatalog {
	t.Helper()
	catalog, err := NewDocumentCatalog(store)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func stageDocumentContent(
	t *testing.T,
	content *local.ContentStore,
	value document.Document,
	data []byte,
) document.Location {
	t.Helper()
	staged, err := content.Stage(testZoneContext(t))
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()
	if _, err := staged.Write(data); err != nil {
		t.Fatal(err)
	}
	location, err := staged.Publish(testZoneContext(t), value)
	if err != nil {
		t.Fatal(err)
	}
	staged.Confirm()
	return location
}

type failingCorpusProducer struct {
	failure error
}

func (p failingCorpusProducer) Publish(context.Context, []corpus.Event) error {
	return p.failure
}

func assertDocumentCounts(
	t *testing.T,
	database *sql.DB,
	documents int,
	zoneDocuments int,
	events int,
) {
	t.Helper()
	for table, expected := range map[string]int{
		"documents": documents, "zone_documents": zoneDocuments, "journal_events": events,
	} {
		var count int
		if err := database.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != expected {
			t.Fatalf("%s count = %d, want %d", table, count, expected)
		}
	}
}
