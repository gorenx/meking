package sqlite

import (
	"testing"

	corpusmerger "github.com/memoria-space/meking/corpus/zonemerger"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

const childMergeZoneID = "20000000-0000-4000-8000-000000000002"

func TestChildCorporaBoundaryCombinesAcceptedInputs(t *testing.T) {
	database := openCorpusDatabase(t)
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	application, err := corpusmerger.NewBoundaryApplication(transactions, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := zone.WithChildZone(testZoneContext(t), childMergeZoneID)
	if err != nil {
		t.Fatal(err)
	}
	for _, sourceCorporaID := range []string{"7", "9", "7"} {
		if err := application.MergeChildCorpora(ctx, sourceCorporaID); err != nil {
			t.Fatalf("MergeChildCorpora(%s): %v", sourceCorporaID, err)
		}
	}
	var stored int64
	if err := database.QueryRow(`
		SELECT source_corpora_id FROM corpus_child_boundaries
		WHERE zone_id = ? AND child_zone_id = ?`,
		"10000000-0000-4000-8000-000000000001", childMergeZoneID,
	).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 9 {
		t.Fatalf("stored Child Corpora boundary = %d, want 9", stored)
	}
}
