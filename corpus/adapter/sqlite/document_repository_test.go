package sqlite

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	localdocument "github.com/memoria-space/meking/corpus/adapter/local"
	"github.com/memoria-space/meking/corpus/document"
)

func TestDocumentRepositoryDeduplicatesDigestAndKeepsFirstLocation(t *testing.T) {
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
	store, err := NewStore(database)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repository, err := NewDocumentRepository(store)
	if err != nil {
		t.Fatalf("NewDocumentRepository: %v", err)
	}
	root := t.TempDir()
	content, err := localdocument.NewContentStore(root)
	if err != nil {
		t.Fatalf("NewContentStore: %v", err)
	}
	manager, err := document.NewManager(repository, content)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	preparedUpload, err := manager.PrepareUpload(testZoneContext(t), document.UploadCommand{
		Name: "source.txt", MediaType: "text/plain",
		Content: bytes.NewBufferString("knowledge"),
	})
	if err != nil {
		t.Fatalf("PrepareUpload: %v", err)
	}
	first, err := manager.Record(testZoneContext(t), preparedUpload.LocatedDocument)
	if err != nil {
		t.Fatalf("Record prepared upload: %v", err)
	}
	preparedUpload.Confirm()
	if err := os.WriteFile(filepath.Join(root, "alias.txt"), []byte("knowledge"), 0o600); err != nil {
		t.Fatalf("write alias: %v", err)
	}
	prepared, err := manager.PrepareExisting(testZoneContext(t), document.StoredContent{
		Name: "alias.txt", MediaType: "text/plain", Location: "alias.txt",
	})
	if err != nil {
		t.Fatalf("PrepareExisting: %v", err)
	}
	second, err := manager.Record(testZoneContext(t), prepared)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, err := repository.Get(testZoneContext(t), first.Document.ID); !errors.Is(err, document.ErrNotFound) {
		t.Fatalf("Get before Zone selection error = %v, want ErrNotFound", err)
	}
	if _, err := manager.Associate(testZoneContext(t), []document.ID{first.Document.ID}); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	stored, err := repository.Get(testZoneContext(t), first.Document.ID)
	if err != nil {
		t.Fatalf("Get after Zone selection: %v", err)
	}
	var documentCount int
	if err := database.QueryRow("SELECT count(*) FROM documents").Scan(&documentCount); err != nil {
		t.Fatalf("count Documents: %v", err)
	}
	if first.Document.ID != second.Document.ID || documentCount != 1 ||
		second.Location != first.Location || stored.Location != first.Location {
		t.Fatalf("first=%#v second=%#v documents=%d stored=%#v", first, second, documentCount, stored)
	}
}
