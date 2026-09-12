package sqlite

import (
	"context"
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
)

func TestTextChunkingProgressBuildsOneCorpusAndAdvancesAfterActivation(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	firstText := saveCorpusText(t, store, "alpha")
	secondText := saveCorpusText(t, store, "bravo")
	firstUnit, _ := textunits.NewTextUnitBody(firstText.Body)
	firstID, firstProgress, err := beginTextChunking(t, store, "first-text", firstText.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondUnit, _ := textunits.NewTextUnitBody(secondText.Body)
	secondID, secondProgress, err := beginTextChunking(t, store, "second-text", secondText.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondID != firstID {
		t.Fatalf("Building Corpus IDs = %s then %s, want one ID", firstID, secondID)
	}
	if err := saveTextUnitBatch(testZoneContext(t), store, firstID, firstProgress, []textunits.TextUnit{{
		TextUnit:   firstUnit,
		StartIndex: 0, EndIndex: len([]rune(firstText.Body)), TokenCount: 1,
	}}, true); err != nil {
		t.Fatal(err)
	}
	if _, ready, err := store.FinalizeCorpora(testZoneContext(t), firstID); err != nil || ready {
		t.Fatalf("FinalizeCorpora(first Text) = ready %t, error %v; want incomplete", ready, err)
	}

	if err := saveTextUnitBatch(testZoneContext(t), store, secondID, secondProgress, []textunits.TextUnit{{
		TextUnit:   secondUnit,
		StartIndex: 0, EndIndex: len([]rune(secondText.Body)), TokenCount: 1,
	}}, true); err != nil {
		t.Fatal(err)
	}
	set, ready, err := store.FinalizeCorpora(testZoneContext(t), secondID)
	if err != nil || !ready || len(set.TextUnits()) != 2 {
		t.Fatalf("FinalizeCorpora() = (%#v, %t, %v)", set, ready, err)
	}
	if err := store.Activate(testZoneContext(t), set.ID); err != nil {
		t.Fatal(err)
	}

	nextID, _, err := beginTextChunking(t, store, "next-corpus", firstText.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nextID != "2" {
		t.Fatalf("next Corpus ID = %s, want 2", nextID)
	}
}

func TestTextChunkingProgressReplayKeepsPosition(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	value := saveCorpusText(t, store, "alpha")
	unit, _ := textunits.NewTextUnitBody(value.Body)
	id, progress, err := beginTextChunking(t, store, "text-prepared", value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveTextUnitBatch(testZoneContext(t), store, id, progress, []textunits.TextUnit{{
		TextUnit:   unit,
		StartIndex: 0, EndIndex: len([]rune(value.Body)), TokenCount: 1,
	}}, true); err != nil {
		t.Fatal(err)
	}
	replayedID, replayed, err := beginTextChunking(t, store, "text-prepared", value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replayedID != id || !replayed.Completed || replayed.NextChunkIndex != 1 {
		t.Fatalf("replayed progress = (%s, %#v), want completed position 1 for %s", replayedID, replayed, id)
	}
}

func TestNewLocalCorporaDoesNotInheritChildOnlyText(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	value := saveCorpusText(t, store, "child text")
	unit, _ := textunits.NewTextUnitBody(value.Body)
	id, progress, err := beginTextChunking(t, store, "child-text", value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveTextUnitBatch(testZoneContext(t), store, id, progress, []textunits.TextUnit{{
		TextUnit: unit, StartIndex: 0, EndIndex: len([]rune(value.Body)), TokenCount: 2,
	}}, true); err != nil {
		t.Fatal(err)
	}
	if _, ready, err := store.FinalizeCorpora(testZoneContext(t), id); err != nil || !ready {
		t.Fatalf("FinalizeCorpora() ready=%v err=%v", ready, err)
	}
	if err := store.Activate(testZoneContext(t), id); err != nil {
		t.Fatal(err)
	}
	const parentZoneID = "10000000-0000-4000-8000-000000000001"
	if _, err := database.Exec(`DELETE FROM corpus_local_texts WHERE zone_id = ? AND text_id = ?`,
		parentZoneID, string(value.ID)); err != nil {
		t.Fatal(err)
	}
	nextID, err := store.BeginCorporaBuild(testZoneContext(t))
	if err != nil {
		t.Fatal(err)
	}
	var inherited int
	if err := database.QueryRow(`
		SELECT count(*) FROM text_unit_spans
		WHERE zone_id = ? AND corpora_id = ? AND text_id = ?`,
		parentZoneID, corporaIDSequence(nextID), string(value.ID)).Scan(&inherited); err != nil {
		t.Fatal(err)
	}
	if inherited != 0 {
		t.Fatalf("child-only Text inherited spans = %d, want 0", inherited)
	}
}

func beginTextChunking(
	t *testing.T,
	store *Store,
	source corpus.EventID,
	textID text.ID,
) (corpus.CorporaID, corpus.TextChunkingProgress, error) {
	t.Helper()
	id, err := store.BeginCorporaBuild(testZoneContext(t))
	if err != nil {
		return "", corpus.TextChunkingProgress{}, err
	}
	progress, err := store.OpenTextChunkingProgress(testZoneContext(t), id, source, textID)
	return id, progress, err
}

func saveTextUnitBatch(
	ctx context.Context,
	store *Store,
	corporaID corpus.CorporaID,
	progress corpus.TextChunkingProgress,
	spans []textunits.TextUnit,
	complete bool,
) error {
	if err := store.SaveTextUnitBatch(ctx, corporaID, progress.TextID, spans); err != nil {
		return err
	}
	return store.AdvanceTextChunkingProgress(
		ctx, corporaID, progress, progress.NextChunkIndex+len(spans), complete,
	)
}
