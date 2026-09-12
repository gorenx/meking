package adapter_test

import (
	"database/sql"
	"log/slog"
	"net/url"
	"path/filepath"
	"testing"

	querybasicadapter "github.com/memoria-space/meking/query/basic/adapter"
	"github.com/memoria-space/meking/semantic"
	semanticsqlite "github.com/memoria-space/meking/semantic/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
	_ "modernc.org/sqlite"
)

func TestVectorStoreOpensPinnedTextUnitNamespace(t *testing.T) {
	ctx, err := zone.NewContext(t.Context(), "10000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	store := openVectorStore(t, filepath.Join(t.TempDir(), "vectors.sqlite"))
	if err := store.Add(ctx, semantic.Namespace(adapterCorporaID), []semantic.Vector{
		{ID: "unit-a", Values: []float64{1, 0}},
		{ID: "unit-b", Values: []float64{0, 1}},
	}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	adapter, err := querybasicadapter.NewVectorStore(store)
	if err != nil {
		t.Fatalf("NewVectorStore() error = %v", err)
	}
	reader, err := adapter.OpenTextUnits(ctx, string(adapterCorporaID))
	if err != nil {
		t.Fatalf("OpenTextUnits() error = %v", err)
	}
	if reader.CorporaID() != string(adapterCorporaID) ||
		reader.Model() != "embedding-model" || reader.Dimension() != 2 {
		t.Fatalf(
			"reader identity/model/dimension = %q/%q/%d",
			reader.CorporaID(), reader.Model(), reader.Dimension(),
		)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("reader Close() error = %v", err)
		}
	})
	if err := store.Delete(ctx, semantic.Namespace(adapterCorporaID)); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	matches, err := reader.Search(ctx, []float64{1, 0}, 1)
	if err != nil || len(matches) != 1 || matches[0].TextUnitID != "unit-a" {
		t.Fatalf("Search after retirement = %#v, %v", matches, err)
	}
}

func openVectorStore(t *testing.T, path string) *semanticsqlite.Store {
	t.Helper()
	location := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := location.Query()
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(OFF)")
	query.Add("_pragma", "synchronous(FULL)")
	location.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", location.String())
	if err != nil {
		t.Fatalf("open Semantic SQLite: %v", err)
	}
	database.SetMaxOpenConns(9)
	database.SetMaxIdleConns(9)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Semantic SQLite Close() error = %v", err)
		}
	})
	persistence, err := semanticsqlite.NewStore(
		database,
		"embedding-model",
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("semanticsqlite.NewStore() error = %v", err)
	}
	return persistence
}
