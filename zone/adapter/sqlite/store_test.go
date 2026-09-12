package sqlite

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/memoria-space/meking/internal/sqlitepool"
	"github.com/memoria-space/meking/zone"
)

func TestStorePersistsRootAndChildDefinitions(t *testing.T) {
	database, err := sqlitepool.Open(filepath.Join(t.TempDir(), "zone.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	createdAt := time.Date(2026, time.August, 3, 1, 2, 3, 4, time.UTC)
	root, err := zone.NewRootDefinition(
		"10000000-0000-4000-8000-000000000001",
		"user-1",
		createdAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	child, err := zone.NewChildDefinition(
		"10000000-0000-4000-8000-000000000002",
		root.ID,
		createdAt.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(t.Context(), root); err != nil {
		t.Fatalf("Create(root) error = %v", err)
	}
	if err := store.Create(t.Context(), child); err != nil {
		t.Fatalf("Create(child) error = %v", err)
	}
	resolvedRoot, err := store.ResolveRoot(t.Context(), "user-1")
	if err != nil || resolvedRoot != root {
		t.Fatalf("ResolveRoot() = %#v, %v; want %#v", resolvedRoot, err, root)
	}
	resolved, err := store.Resolve(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("Resolve(child) error = %v", err)
	}
	if parent, ok := resolved.Parent(); !ok || parent != root.ID {
		t.Fatalf("resolved Child Parent = %q, %v", parent, ok)
	}
	if resolved.UserID != "" {
		t.Fatalf("resolved Child User ID = %q", resolved.UserID)
	}
	definitions, err := store.List(t.Context())
	if err != nil || len(definitions) != 2 || definitions[0].ID != root.ID {
		t.Fatalf("List() = %#v, %v", definitions, err)
	}
	duplicateRoot, err := zone.NewRootDefinition(
		"10000000-0000-4000-8000-000000000003",
		"user-1",
		createdAt.Add(2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(t.Context(), duplicateRoot); err == nil {
		t.Fatal("Create(duplicate User Root) error = nil")
	}
}

func TestStoreReportsMissingDefinition(t *testing.T) {
	database, err := sqlitepool.Open(filepath.Join(t.TempDir(), "zone.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Resolve(t.Context(), "10000000-0000-4000-8000-000000000001")
	if !errors.Is(err, zone.ErrNotFound) {
		t.Fatalf("Resolve(missing) error = %v", err)
	}
	_, err = store.ResolveRoot(t.Context(), "missing-user")
	if !errors.Is(err, zone.ErrNotFound) {
		t.Fatalf("ResolveRoot(missing) error = %v", err)
	}
}
