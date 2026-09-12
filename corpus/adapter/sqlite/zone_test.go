package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/internal/uuid"
	"github.com/memoria-space/meking/zone"
)

func TestStoreScopesCurrentCorporaAndSequenceByZone(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	zoneA := testZoneContextWithID(t, "10000000-0000-4000-8000-000000000001")
	zoneB := testZoneContextWithID(t, "20000000-0000-4000-8000-000000000002")

	for index, test := range []struct {
		ctx  context.Context
		body string
	}{{ctx: zoneA, body: "alpha"}, {ctx: zoneB, body: "bravo"}} {
		value := saveZoneText(t, test.ctx, store, test.body)
		unit, err := textunits.NewTextUnitBody(value.Body)
		if err != nil {
			t.Fatal(err)
		}
		corporaID, err := store.BeginCorporaBuild(test.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if corporaID != "1" {
			t.Fatalf("Zone %d first CorporaID = %s, want 1", index, corporaID)
		}
		progress, err := store.OpenTextChunkingProgress(
			test.ctx, corporaID, corpus.EventID("text-available"), value.ID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := saveTextUnitBatch(test.ctx, store, corporaID, progress, []textunits.TextUnit{{
			TextUnit: unit, StartIndex: 0, EndIndex: len([]rune(value.Body)), TokenCount: 1,
		}}, true); err != nil {
			t.Fatal(err)
		}
		if _, ready, err := store.FinalizeCorpora(test.ctx, corporaID); err != nil || !ready {
			t.Fatalf("FinalizeCorpora() = ready %t, error %v", ready, err)
		}
		if err := store.Activate(test.ctx, corporaID); err != nil {
			t.Fatal(err)
		}
	}

	for index, ctx := range []context.Context{zoneA, zoneB} {
		current, err := store.CurrentID(ctx)
		if err != nil || current != "1" {
			t.Fatalf("Zone %d CurrentID() = %s, %v; want 1", index, current, err)
		}
	}
}

func TestStoreRejectsMissingZoneContext(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginCorporaBuild(t.Context()); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("BeginCorporaBuild() error = %v, want %v", err, zone.ErrContextRequired)
	}
	if _, err := store.CurrentID(t.Context()); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("CurrentID() error = %v, want %v", err, zone.ErrContextRequired)
	}
}

func testZoneContext(t *testing.T) context.Context {
	t.Helper()
	return testZoneContextWithID(t, "10000000-0000-4000-8000-000000000001")
}

func testZoneContextWithID(t *testing.T, id string) context.Context {
	t.Helper()
	ctx, err := zone.NewContext(t.Context(), zone.ID(id))
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func saveZoneText(t *testing.T, ctx context.Context, store *Store, body string) text.Text {
	t.Helper()
	documentValue, err := document.New(filepath.Base(body)+".txt", "text/plain", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	documents, err := NewDocumentRepository(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := documents.Save(ctx, documentValue, document.Location("memory://"+string(documentValue.ID))); err != nil {
		t.Fatal(err)
	}
	if _, err := documents.Associate(ctx, []document.ID{documentValue.ID}); err != nil {
		t.Fatal(err)
	}
	id, err := uuid.NewV4()
	if err != nil {
		t.Fatal(err)
	}
	value := text.Text{
		ID: text.ID(id), DocumentID: documentValue.ID, Title: "Source", Body: body,
		Format: text.PlainText, ExtractionProfile: "plain/v1", NormalizationProfile: "standard/v1",
	}
	texts, err := NewTextStore(store)
	if err != nil {
		t.Fatal(err)
	}
	value, err = texts.Save(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
