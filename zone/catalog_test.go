package zone

import (
	"context"
	"errors"
	"testing"
	"time"
)

type definitionMemoryStore struct {
	definitions map[ID]Definition
}

func newDefinitionMemoryStore() *definitionMemoryStore {
	return &definitionMemoryStore{definitions: make(map[ID]Definition)}
}

func (store *definitionMemoryStore) Create(_ context.Context, definition Definition) error {
	if _, exists := store.definitions[definition.ID]; exists {
		return errors.New("duplicate Zone")
	}
	store.definitions[definition.ID] = definition
	return nil
}

func (store *definitionMemoryStore) Resolve(_ context.Context, id ID) (Definition, error) {
	definition, exists := store.definitions[id]
	if !exists {
		return Definition{}, ErrNotFound
	}
	return definition, nil
}

func (store *definitionMemoryStore) ResolveRoot(_ context.Context, userID string) (Definition, error) {
	for _, definition := range store.definitions {
		if definition.Role == RoleRoot && definition.UserID == userID {
			return definition, nil
		}
	}
	return Definition{}, ErrNotFound
}

func (store *definitionMemoryStore) List(context.Context) ([]Definition, error) {
	definitions := make([]Definition, 0, len(store.definitions))
	for _, definition := range store.definitions {
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func TestCatalogCreatesRootAndDirectChild(t *testing.T) {
	store := newDefinitionMemoryStore()
	catalog, err := NewCatalog(CatalogDependencies{Definitions: store})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	root, err := catalog.CreateRoot(t.Context())
	if err != nil {
		t.Fatalf("CreateRoot() error = %v", err)
	}
	child, err := catalog.CreateChild(t.Context(), root.ID)
	if err != nil {
		t.Fatalf("CreateChild() error = %v", err)
	}
	if parent, err := catalog.ResolveDirectParent(t.Context(), child.ID); err != nil || parent != root.ID {
		t.Fatalf("ResolveDirectParent() = %q, %v", parent, err)
	}
	if _, err := catalog.CreateChild(t.Context(), child.ID); !errors.Is(err, ErrChildDepth) {
		t.Fatalf("CreateChild(Child) error = %v", err)
	}
	definitions, err := catalog.List(t.Context())
	if err != nil || len(definitions) != 2 {
		t.Fatalf("List() = %#v, %v", definitions, err)
	}
}

func TestCatalogEnsuresUserRootAndSessionChild(t *testing.T) {
	store := newDefinitionMemoryStore()
	catalog, err := NewCatalog(CatalogDependencies{Definitions: store})
	if err != nil {
		t.Fatal(err)
	}
	root, err := catalog.Root(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	repeatedRoot, err := catalog.Root(t.Context(), "user-1")
	if err != nil || repeatedRoot != root || len(store.definitions) != 1 {
		t.Fatalf("repeated Root() = %#v, %v; definitions = %d", repeatedRoot, err, len(store.definitions))
	}

	child, err := catalog.EnsureChild(t.Context(), root.ID, childID)
	if err != nil {
		t.Fatalf("EnsureChild() error = %v", err)
	}
	if child.ID != childID {
		t.Fatalf("EnsureChild() ID = %q, want %q", child.ID, childID)
	}
	repeatedChild, err := catalog.EnsureChild(t.Context(), root.ID, childID)
	if err != nil || repeatedChild != child || len(store.definitions) != 2 {
		t.Fatalf("repeated EnsureChild() = %#v, %v; definitions = %d", repeatedChild, err, len(store.definitions))
	}

	otherRoot, err := catalog.Root(t.Context(), "user-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.EnsureChild(t.Context(), otherRoot.ID, childID); !errors.Is(err, ErrParentConflict) {
		t.Fatalf("EnsureChild(other Root) error = %v, want %v", err, ErrParentConflict)
	}
	if _, err := catalog.EnsureChild(t.Context(), root.ID, root.ID); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("EnsureChild(Root ID) error = %v, want %v", err, ErrInvalidDefinition)
	}
	if _, err := catalog.Root(t.Context(), " user-1"); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("Root(invalid User) error = %v, want %v", err, ErrInvalidUser)
	}
}

func TestCatalogRejectsMissingDependenciesAndInvalidStoredFacts(t *testing.T) {
	if _, err := NewCatalog(CatalogDependencies{}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("NewCatalog(empty) error = %v", err)
	}
	store := newDefinitionMemoryStore()
	store.definitions[rootID] = Definition{ID: rootID, Role: RoleChild, CreatedAt: time.Now().UTC()}
	catalog, err := NewCatalog(CatalogDependencies{Definitions: store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Resolve(t.Context(), rootID); !errors.Is(err, ErrParentRequired) {
		t.Fatalf("Resolve(invalid stored Definition) error = %v", err)
	}
}

func TestCatalogPropagatesCancellation(t *testing.T) {
	store := newDefinitionMemoryStore()
	catalog, err := NewCatalog(CatalogDependencies{Definitions: store})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := catalog.CreateRoot(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateRoot(cancelled) error = %v", err)
	}
}
