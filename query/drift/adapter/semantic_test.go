package adapter

import (
	"database/sql"
	"log/slog"
	"net/url"
	"path/filepath"
	"testing"

	querydrift "github.com/memoria-space/meking/query/drift"
	"github.com/memoria-space/meking/semantic"
	semanticsqlite "github.com/memoria-space/meking/semantic/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
	_ "modernc.org/sqlite"
)

func TestSemanticAdapterSearchesExactReportSet(t *testing.T) {
	ctx, err := zone.NewContext(t.Context(), "10000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	store := openReportVectorStore(t)
	const reportSetID = "10000000-0000-4000-8000-000000000001"
	if err := store.Add(ctx, semantic.Namespace(reportSetID), []semantic.Vector{
		{ID: "report-a", Values: []float64{1, 0}},
		{ID: "report-b", Values: []float64{0, 1}},
	}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	vectors, err := NewReportVectorStore(store)
	if err != nil {
		t.Fatalf("NewReportVectorStore() error = %v", err)
	}
	reader, err := vectors.OpenReports(ctx, reportSetID)
	if err != nil {
		t.Fatalf("OpenReports() error = %v", err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("reader Close() error = %v", err)
		}
	})
	matches, err := reader.Search(ctx, []float64{1, 0}, 2, []string{"report-a"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	want := []querydrift.ReportMatch{
		{ReportID: "report-a", Score: 1},
	}
	if reader.ReportSetID() != reportSetID || reader.Model() != "embedding-model" ||
		reader.Dimension() != 2 || len(matches) != len(want) ||
		matches[0] != want[0] {
		t.Fatalf("reader/matches = %q/%q/%d %#v", reader.ReportSetID(), reader.Model(), reader.Dimension(), matches)
	}
}

func openReportVectorStore(t *testing.T) *semanticsqlite.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report-vectors.sqlite")
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
