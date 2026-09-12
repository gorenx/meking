package local

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
)

func TestReconcilerDeduplicatesDigestAndReportsChangedAndMissing(t *testing.T) {
	root := t.TempDir()
	content, err := NewContentStore(root)
	if err != nil {
		t.Fatalf("NewContentStore: %v", err)
	}
	documents := newReconcilerDocuments(root)
	reconciler, err := NewReconciler(content, documents, `.*\.txt$`)
	if err != nil {
		t.Fatalf("NewReconciler: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "manual.txt"), []byte("first"), 0o600); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.md"), []byte("ignored"), 0o600); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}
	first, err := reconciler.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	if len(first.DocumentsAdded) != 1 || len(first.Duplicates) != 0 || len(documents.byLocation) != 1 {
		t.Fatalf("first result = %#v", first)
	}
	documentID := first.DocumentsAdded[0]

	if err := os.WriteFile(filepath.Join(root, "alias.txt"), []byte("first"), 0o600); err != nil {
		t.Fatalf("write alias: %v", err)
	}
	alias, err := reconciler.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("alias Reconcile: %v", err)
	}
	if len(alias.DocumentsAdded) != 0 || len(alias.Duplicates) != 1 ||
		alias.Duplicates[0].DocumentID != documentID || alias.Duplicates[0].Location != "alias.txt" {
		t.Fatalf("alias result = %#v locations=%#v", alias, documents.byLocation)
	}
	if err := os.Remove(filepath.Join(root, "alias.txt")); err != nil {
		t.Fatalf("remove alias: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "manual.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatalf("change canonical file: %v", err)
	}
	changed, err := reconciler.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("changed Reconcile: %v", err)
	}
	if len(changed.Changed) != 1 || changed.Changed[0].DocumentID != documentID {
		t.Fatalf("changed result = %#v", changed)
	}

	if err := os.Remove(filepath.Join(root, "manual.txt")); err != nil {
		t.Fatalf("remove local file: %v", err)
	}
	missing, err := reconciler.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("missing Reconcile: %v", err)
	}
	if len(missing.Missing) != 1 || missing.Missing[0].Location != "manual.txt" {
		t.Fatalf("missing result = %#v", missing)
	}
}

type reconcilerDocuments struct {
	root       string
	documents  map[document.ID]document.Document
	byDigest   map[string]document.ID
	byLocation map[document.Location]document.ID
}

func newReconcilerDocuments(root string) *reconcilerDocuments {
	return &reconcilerDocuments{
		root: root, documents: make(map[document.ID]document.Document),
		byDigest: make(map[string]document.ID), byLocation: make(map[document.Location]document.ID),
	}
}

func (s *reconcilerDocuments) RegisterDocument(
	_ context.Context,
	registration corpus.DocumentRegistration,
) (corpus.RegisteredDocument, error) {
	data, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(string(registration.Location))))
	if err != nil {
		return corpus.RegisteredDocument{}, err
	}
	candidate, err := document.New(registration.Name, registration.MediaType, data)
	if err != nil {
		return corpus.RegisteredDocument{}, err
	}
	value := candidate
	documentCreated := false
	if id, found := s.byDigest[candidate.Digest]; found {
		value = s.documents[id]
		return corpus.RegisteredDocument{DocumentID: value.ID, ContentDigest: value.Digest}, nil
	} else {
		s.documents[value.ID] = value
		s.byDigest[value.Digest] = value.ID
		documentCreated = true
	}
	s.byLocation[registration.Location] = value.ID
	return corpus.RegisteredDocument{
		DocumentID: value.ID, ContentDigest: value.Digest,
		DocumentCreated: documentCreated,
	}, nil
}

func (s *reconcilerDocuments) LocatedDocument(
	_ context.Context,
	location document.Location,
) (document.LocatedDocument, error) {
	id, found := s.byLocation[location]
	if !found {
		return document.LocatedDocument{}, document.ErrNotFound
	}
	return document.LocatedDocument{Document: s.documents[id], Location: location}, nil
}

func (s *reconcilerDocuments) ListLocatedDocuments(
	_ context.Context,
	page document.Page,
) (document.LocatedDocumentPage, error) {
	values := make([]document.LocatedDocument, 0, len(s.byLocation))
	for location, id := range s.byLocation {
		values = append(values, document.LocatedDocument{Document: s.documents[id], Location: location})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Location < values[j].Location })
	if page.Offset >= len(values) {
		return document.LocatedDocumentPage{}, nil
	}
	end := min(page.Offset+page.Limit, len(values))
	result := document.LocatedDocumentPage{Documents: values[page.Offset:end]}
	if end < len(values) {
		result.Next = &document.Page{Offset: end, Limit: page.Limit}
	}
	return result, nil
}
