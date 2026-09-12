package sqlite_test

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"path/filepath"
	"testing"

	semanticsqlite "github.com/memoria-space/meking/semantic/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
	_ "modernc.org/sqlite"
)

const semanticTestZoneID zone.ID = "10000000-0000-4000-8000-000000000001"

func testContext(t *testing.T) context.Context {
	return testZoneContext(t, semanticTestZoneID)
}

func testZoneContext(t *testing.T, id zone.ID) context.Context {
	t.Helper()
	ctx, err := zone.NewContext(t.Context(), id)
	if err != nil {
		t.Fatalf("NewContext(%q): %v", id, err)
	}
	return ctx
}

func openStore(t *testing.T, path string) (*semanticsqlite.Store, *sql.DB) {
	t.Helper()
	database := openDatabase(t, path)
	store, err := semanticsqlite.NewStore(
		database,
		"embedding-test",
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, database
}

func openDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	location := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := location.Query()
	query.Set("mode", "rwc")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(OFF)")
	query.Add("_pragma", "synchronous(FULL)")
	location.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", location.String())
	if err != nil {
		t.Fatalf("open Semantic database: %v", err)
	}
	database.SetMaxOpenConns(9)
	database.SetMaxIdleConns(9)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close Semantic database: %v", err)
		}
	})
	return database
}
