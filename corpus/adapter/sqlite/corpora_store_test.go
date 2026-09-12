package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/internal/uuid"
)

func TestStorePersistsCorporaRelationshipsWithoutForeignKeys(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	var foreignKeyDDL int
	if err := database.QueryRow(`
		SELECT count(*) FROM sqlite_schema
		WHERE sql IS NOT NULL AND upper(sql) LIKE '%REFERENCES%'
	`).Scan(&foreignKeyDDL); err != nil {
		t.Fatalf("inspect schema foreign keys: %v", err)
	}
	if foreignKeyDDL != 0 {
		t.Fatalf("schema contains %d foreign key declarations", foreignKeyDDL)
	}

	textValue := saveCorpusText(t, store, "same same")
	unit, err := textunits.NewTextUnitBody("same")
	if err != nil {
		t.Fatalf("NewTextUnitBody() error = %v", err)
	}
	chunked, err := corpus.NewChunkedText(
		textValue.ID,
		[]textunits.TextUnit{
			{TextUnit: unit, StartIndex: 0, EndIndex: 4, TokenCount: 1},
			{TextUnit: unit, StartIndex: 5, EndIndex: 9, TokenCount: 1},
		},
	)
	if err != nil {
		t.Fatalf("NewChunkedText() error = %v", err)
	}
	set := prepareStoredCorpora(t, store, "first-corpus", textValue, chunked.TextUnits)
	if err := store.Activate(testZoneContext(t), set.ID); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	loaded, err := store.Current(testZoneContext(t))
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	loadedUnits := loaded.TextUnits()
	if loaded.TextCount() != 1 || len(loadedUnits) != 2 || loadedUnits[1].StartIndex != 5 {
		t.Fatalf("loaded Corpora = %#v", loaded)
	}
	locations, err := store.TextUnitLocations(testZoneContext(t), set.ID, []textunits.TextUnitID{unit.ID})
	if err != nil {
		t.Fatalf("TextUnitLocations() error = %v", err)
	}
	if len(locations) != 2 || locations[0].TextUnit.StartIndex != 0 ||
		locations[1].TextUnit.StartIndex != 5 ||
		locations[0].DocumentLocation == "" {
		t.Fatalf("TextUnit locations = %#v", locations)
	}
	replacement, err := corpus.NewChunkedText(textValue.ID, set.TextUnits())
	if err != nil {
		t.Fatal(err)
	}
	replacementSet := prepareStoredCorpora(t, store, "replacement-corpus", textValue, replacement.TextUnits)
	if err := store.Activate(testZoneContext(t), replacementSet.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(testZoneContext(t), set.ID); !errors.Is(err, corpus.ErrCorporaActivationConflict) {
		t.Fatalf("reactivate replaced Corpora error = %v, want ErrCorporaActivationConflict", err)
	}
	if current, err := store.CurrentID(testZoneContext(t)); err != nil || current != replacementSet.ID {
		t.Fatalf("CurrentID() = (%q, %v), want %q", current, err, replacementSet.ID)
	}
}

func TestStoreDetectsOrphanTextUnitSpan(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	textValue := saveCorpusText(t, store, "source")
	unit, _ := textunits.NewTextUnitBody("source")
	chunked, err := corpus.NewChunkedText(
		textValue.ID,
		[]textunits.TextUnit{{
			TextUnit:   unit,
			StartIndex: 0, EndIndex: 6, TokenCount: 1,
		}},
	)
	if err != nil {
		t.Fatalf("NewChunkedText() error = %v", err)
	}
	set := prepareStoredCorpora(t, store, "orphan-corpus", textValue, chunked.TextUnits)
	if _, err := database.Exec(`
		INSERT INTO text_unit_spans (
			zone_id, corpora_id, text_id, text_unit_id, start_char, end_char, token_count
		) VALUES (?, ?, ?, 'missing-unit', 7, 8, 1)
	`, "10000000-0000-4000-8000-000000000001", set.ID, textValue.ID); err != nil {
		t.Fatalf("insert orphan TextUnitSpan: %v", err)
	}
	if _, err := store.Load(testZoneContext(t), set.ID); !errors.Is(err, corpus.ErrCorpusDataIntegrity) {
		t.Fatalf("Load() error = %v, want ErrCorpusDataIntegrity", err)
	}
}

func prepareStoredCorpora(
	t *testing.T,
	store *Store,
	source corpus.EventID,
	value text.Text,
	spans []textunits.TextUnit,
) corpus.Corpora {
	t.Helper()
	id, progress, err := beginTextChunking(t, store, source, value.ID)
	if err != nil {
		t.Fatalf("beginTextChunking() error = %v", err)
	}
	if err := saveTextUnitBatch(testZoneContext(t), store, id, progress, spans, true); err != nil {
		t.Fatalf("SaveTextUnitBatch() error = %v", err)
	}
	set, ready, err := store.FinalizeCorpora(testZoneContext(t), id)
	if err != nil {
		t.Fatalf("FinalizeCorpora() error = %v", err)
	}
	if !ready {
		t.Fatal("FinalizeCorpora() reported incomplete Corpus")
	}
	return set
}

func openCorpusDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "corpus.db"))
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		t.Fatalf("configure busy timeout: %v", err)
	}
	if _, err := database.Exec("PRAGMA synchronous = FULL"); err != nil {
		t.Fatalf("configure synchronous mode: %v", err)
	}
	return database
}

func saveCorpusText(t *testing.T, store *Store, body string) text.Text {
	t.Helper()
	documentValue, err := document.New("source.txt", "text/plain", []byte(body))
	if err != nil {
		t.Fatalf("document.New() error = %v", err)
	}
	metadata, err := NewDocumentRepository(store)
	if err != nil {
		t.Fatalf("NewDocumentRepository() error = %v", err)
	}
	location := document.Location("memory://" + string(documentValue.ID))
	if _, err := metadata.Save(testZoneContext(t), documentValue, location); err != nil {
		t.Fatalf("save Document: %v", err)
	}
	if _, err := metadata.Associate(testZoneContext(t), []document.ID{documentValue.ID}); err != nil {
		t.Fatalf("associate Document: %v", err)
	}
	id, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("create Text ID: %v", err)
	}
	value := text.Text{
		ID:                   text.ID(id),
		DocumentID:           documentValue.ID,
		Title:                "Source",
		Body:                 body,
		Format:               text.PlainText,
		ExtractionProfile:    "plain/v1",
		NormalizationProfile: "standard/v1",
	}
	texts, err := NewTextStore(store)
	if err != nil {
		t.Fatalf("NewTextStore() error = %v", err)
	}
	value, err = texts.Save(testZoneContext(t), value)
	if err != nil {
		t.Fatalf("save Text: %v", err)
	}
	return value
}
