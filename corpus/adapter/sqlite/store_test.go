package sqlite

import (
	"testing"

	journalsqlite "github.com/memoria-space/meking/journal/adapter/sqlite"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

func TestStoreSharesProjectDatabaseWithJournal(t *testing.T) {
	database := openCorpusDatabase(t)
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatalf("create transaction scope: %v", err)
	}
	if _, err := journalsqlite.NewStore(database, transactions); err != nil {
		t.Fatalf("create Journal Store: %v", err)
	}
	if _, err := NewStore(database); err != nil {
		t.Fatalf("create Corpus Store after Journal: %v", err)
	}
	for _, name := range []string{"journal_events", "documents", "corpus_schema"} {
		var count int
		if err := database.QueryRow(
			`SELECT count(*) FROM sqlite_schema WHERE name = ?`,
			name,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("SQLite object %q count = %d, want 1", name, count)
		}
	}
}

func TestStoreRejectsOlderCorpusSchema(t *testing.T) {
	database := openCorpusDatabase(t)
	if _, err := database.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE corpus_schema SET version = 14 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(database); err == nil {
		t.Fatal("NewStore() accepted an older Corpus schema")
	}
}
