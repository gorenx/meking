package adapter

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/memoria-space/meking/knowledge"
	knowledgevector "github.com/memoria-space/meking/knowledge/vectorindex"
	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
	"github.com/memoria-space/meking/semantic"
)

const (
	entityOneRef = "11111111-1111-4111-8111-111111111111@4"
	entityTwoRef = "22222222-2222-4222-8222-222222222222@2"
)

type entityNamespaceOpener struct {
	reader    semantic.NamespaceReader
	namespace semantic.Namespace
}

func (s *entityNamespaceOpener) Open(
	_ context.Context,
	namespace semantic.Namespace,
) (semantic.NamespaceReader, error) {
	s.namespace = namespace
	return s.reader, nil
}

type fixedEntityNamespace struct {
	info    semantic.Info
	matches []semantic.Match
	closed  int
}

func (r *fixedEntityNamespace) Info(context.Context) (semantic.Info, error) {
	return r.info, nil
}

func (r *fixedEntityNamespace) IDs(context.Context, string, int) (semantic.IDPage, error) {
	return semantic.IDPage{}, nil
}

func (r *fixedEntityNamespace) Search(
	context.Context,
	[]float64,
	int,
	map[string]string,
) ([]semantic.Match, error) {
	return append([]semantic.Match(nil), r.matches...), nil
}

func (r *fixedEntityNamespace) Close() error {
	r.closed++
	return nil
}

func TestEntityVectorStoreMapsFixedSemanticReferences(t *testing.T) {
	entities := []knowledge.Reference[knowledge.EntityID]{
		{ID: "11111111-1111-4111-8111-111111111111", Version: 4},
		{ID: "22222222-2222-4222-8222-222222222222", Version: 2},
	}
	namespaceID, err := knowledgevector.Namespace(entities)
	if err != nil {
		t.Fatal(err)
	}
	namespace := semantic.Namespace(namespaceID)
	reader := &fixedEntityNamespace{
		info: semantic.Info{Namespace: namespace, Model: "embedding-model", Dimension: 2},
		matches: []semantic.Match{
			{ID: entityTwoRef, Score: 0.75},
			{ID: entityOneRef, Score: 0.5},
		},
	}
	opener := &entityNamespaceOpener{reader: reader}
	store, err := NewEntityVectorStore(opener)
	if err != nil {
		t.Fatalf("NewEntityVectorStore() error = %v", err)
	}
	fixed, err := store.Open(t.Context(), entities)
	if err != nil {
		t.Fatalf("OpenThrough() error = %v", err)
	}
	if opener.namespace != namespace || fixed.Model() != "embedding-model" || fixed.Dimension() != 2 {
		t.Fatalf("fixed metadata = %q/%d; namespace = %q", fixed.Model(), fixed.Dimension(), opener.namespace)
	}
	matches, err := fixed.Search(t.Context(), []float64{1, 0}, 2)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !reflect.DeepEqual(matches, []querylocal.EntityMatch{
		{ID: "22222222-2222-4222-8222-222222222222", Version: 2, Score: 0.75},
		{ID: "11111111-1111-4111-8111-111111111111", Version: 4, Score: 0.5},
	}) {
		t.Fatalf("Search() = %#v", matches)
	}
	if err := fixed.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := fixed.Close(); err != nil || reader.closed != 1 {
		t.Fatalf("idempotent Close() = %v, closes = %d", err, reader.closed)
	}
	_, err = fixed.Search(t.Context(), []float64{1, 0}, 1)
	assertLocalAdapterFailure(t, err, querybase.FailureInternal)
}

func TestEntityVectorStoreRejectsInvalidSemanticReference(t *testing.T) {
	entities := []knowledge.Reference[knowledge.EntityID]{
		{ID: "11111111-1111-4111-8111-111111111111", Version: 4},
	}
	namespaceID, err := knowledgevector.Namespace(entities)
	if err != nil {
		t.Fatal(err)
	}
	namespace := semantic.Namespace(namespaceID)
	reader := &fixedEntityNamespace{
		info:    semantic.Info{Namespace: namespace, Model: "embedding-model", Dimension: 2},
		matches: []semantic.Match{{ID: "not-an-entity-reference", Score: 1}},
	}
	store, err := NewEntityVectorStore(&entityNamespaceOpener{reader: reader})
	if err != nil {
		t.Fatalf("NewEntityVectorStore() error = %v", err)
	}
	fixed, err := store.Open(t.Context(), entities)
	if err != nil {
		t.Fatalf("OpenThrough() error = %v", err)
	}
	defer fixed.Close()
	_, err = fixed.Search(t.Context(), []float64{1, 0}, 1)
	assertLocalAdapterFailure(t, err, querybase.FailurePublicationIncomplete)
}

func TestEntityVectorStoreRejectsWrongNamespace(t *testing.T) {
	entities := []knowledge.Reference[knowledge.EntityID]{
		{ID: "11111111-1111-4111-8111-111111111111", Version: 4},
	}
	reader := &fixedEntityNamespace{info: semantic.Info{
		Namespace: "13", Model: "embedding-model", Dimension: 2,
	}}
	store, err := NewEntityVectorStore(&entityNamespaceOpener{reader: reader})
	if err != nil {
		t.Fatalf("NewEntityVectorStore() error = %v", err)
	}
	_, err = store.Open(t.Context(), entities)
	assertLocalAdapterFailure(t, err, querybase.FailurePublicationIncomplete)
	if reader.closed != 1 {
		t.Fatalf("rejected reader closes = %d", reader.closed)
	}
}

func assertLocalAdapterFailure(t *testing.T, err error, want querybase.FailureCategory) {
	t.Helper()
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != want {
		t.Fatalf("error = %v, want category %s", err, want)
	}
}
