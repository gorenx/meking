package sqlite

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
	corpusmerger "github.com/memoria-space/meking/corpus/zonemerger"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

func TestChildMergeReusesPublishedCorporaWithoutReadingDocumentContent(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := document.NewManager(store.DocumentRepository(), rejectingContentStore{})
	if err != nil {
		t.Fatal(err)
	}
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := corpus.NewService(
		store,
		corpus.WithDocument(documents),
		corpus.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		corpus.WithChunking(textunits.Chunking{
			Type: textunits.TokenChunking, Size: 32, EncodingModel: textunits.DefaultEncodingModel,
		}),
		corpus.WithEventPublishing(transactions, noOpCorpusProducer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	boundaries, err := corpusmerger.NewBoundaryApplication(transactions, store)
	if err != nil {
		t.Fatal(err)
	}
	merger, err := corpusmerger.New(corpusmerger.Dependencies{
		Boundaries: store,
		Corpora:    service,
		Texts:      store.TextStore(),
		Documents:  documents,
		Target:     service,
	})
	if err != nil {
		t.Fatal(err)
	}

	parentContext := testZoneContext(t)
	childContext := testZoneContextWithID(t, childMergeZoneID)
	childCorpora := createPublishedChildCorpora(t, store, documents, childContext)
	mergeContext, err := zone.WithChildZone(parentContext, childMergeZoneID)
	if err != nil {
		t.Fatal(err)
	}
	if err := boundaries.MergeChildCorpora(mergeContext, string(childCorpora.ID)); err != nil {
		t.Fatal(err)
	}

	if err := merger.MergeChildDocuments(parentContext); err != nil {
		t.Fatal(err)
	}
	if err := merger.MergeChildTexts(parentContext); err != nil {
		t.Fatal(err)
	}
	if err := merger.MergeTextUnits(parentContext); err != nil {
		t.Fatal(err)
	}
	if err := merger.MergeTextUnits(parentContext); err != nil {
		t.Fatalf("replay Child TextUnits: %v", err)
	}

	childText := childCorpora.Texts[0]
	storedText, err := store.TextStore().Get(parentContext, childText.TextID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := documents.Get(parentContext, storedText.DocumentID); err != nil {
		t.Fatalf("read merged Document metadata: %v", err)
	}
	building, err := store.LoadBuildingCorpora(parentContext, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(building.Texts) != 1 || len(building.Texts[0].TextUnits) != 1 ||
		building.Texts[0].TextUnits[0].TextUnit.Text != storedText.Body {
		t.Fatalf("merged Building Corpora = %#v", building)
	}
	if _, err := database.Exec(`
		UPDATE text_unit_spans SET token_count = token_count + 1
		WHERE zone_id = ? AND corpora_id = ? AND text_id = ?`,
		"10000000-0000-4000-8000-000000000001", corporaIDSequence(building.ID), string(storedText.ID),
	); err != nil {
		t.Fatal(err)
	}
	if err := merger.MergeTextUnits(parentContext); !errors.Is(err, corpus.ErrCorporaPreparationConflict) {
		t.Fatalf("MergeTextUnits(conflict) error = %v", err)
	}
	var progressCount int
	if err := database.QueryRow(
		"SELECT count(*) FROM text_chunking_progress WHERE zone_id = ?",
		"10000000-0000-4000-8000-000000000001",
	).Scan(&progressCount); err != nil {
		t.Fatal(err)
	}
	if progressCount != 0 {
		t.Fatalf("Child merge Text chunking progress rows = %d, want 0", progressCount)
	}
	var localTextCount int
	if err := database.QueryRow(
		"SELECT count(*) FROM corpus_local_texts WHERE zone_id = ? AND text_id = ?",
		"10000000-0000-4000-8000-000000000001",
		string(storedText.ID),
	).Scan(&localTextCount); err != nil {
		t.Fatal(err)
	}
	if localTextCount != 1 {
		t.Fatalf("Child merge local Text rows = %d, want 1", localTextCount)
	}
	var storedTextCount int
	if err := database.QueryRow(
		"SELECT count(*) FROM texts WHERE id = ?",
		string(storedText.ID),
	).Scan(&storedTextCount); err != nil {
		t.Fatal(err)
	}
	if storedTextCount != 1 {
		t.Fatalf("stored Child Text rows = %d, want 1", storedTextCount)
	}
}

func createPublishedChildCorpora(
	t *testing.T,
	store *Store,
	documents *document.Manager,
	ctx context.Context,
) corpus.Corpora {
	t.Helper()
	const body = "child knowledge"
	documentValue, err := document.New("child.txt", "text/plain", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := documents.Record(ctx, document.LocatedDocument{
		Document: documentValue,
		Location: "child/content/knowledge",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := documents.Associate(ctx, []document.ID{documentValue.ID}); err != nil {
		t.Fatal(err)
	}
	textValue := text.Text{
		ID:                   "30000000-0000-4000-8000-000000000003",
		DocumentID:           documentValue.ID,
		Title:                "Child knowledge",
		Body:                 body,
		Format:               text.PlainText,
		ExtractionProfile:    "plain-text-v1",
		NormalizationProfile: "standard-v1",
	}
	if _, err := store.TextStore().Save(ctx, textValue); err != nil {
		t.Fatal(err)
	}
	unit, err := textunits.NewTextUnitBody(body)
	if err != nil {
		t.Fatal(err)
	}
	corporaID, err := store.BeginCorporaBuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := store.OpenTextChunkingProgress(ctx, corporaID, "child-text-prepared", textValue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveTextUnitBatch(ctx, store, corporaID, progress, []textunits.TextUnit{{
		TextUnit: unit, StartIndex: 0, EndIndex: len([]rune(body)), TokenCount: 2,
	}}, true); err != nil {
		t.Fatal(err)
	}
	value, ready, err := store.FinalizeCorpora(ctx, corporaID)
	if err != nil || !ready {
		t.Fatalf("FinalizeCorpora() = (%#v, %t, %v)", value, ready, err)
	}
	if err := store.Activate(ctx, corporaID); err != nil {
		t.Fatal(err)
	}
	return value
}

type rejectingContentStore struct{}

type noOpCorpusProducer struct{}

func (noOpCorpusProducer) Publish(context.Context, []corpus.Event) error {
	return nil
}

func (rejectingContentStore) Stage(context.Context) (document.StagedContent, error) {
	return nil, errors.New("Document content must not be staged during Child merge")
}

func (rejectingContentStore) Open(context.Context, document.Location) (io.ReadCloser, error) {
	return nil, errors.New("Document content must not be opened during Child merge")
}
