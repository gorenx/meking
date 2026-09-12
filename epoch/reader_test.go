package epoch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/knowledge"
)

func TestReaderReturnsValidatedCurrentAndExactEpoch(t *testing.T) {
	published := testEpoch(
		3,
		testKnowledgeVersions(8),
		time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	)
	reader, err := NewReader(&readerStore{current: published})
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	current, err := reader.Current(t.Context())
	if err != nil || !current.Equal(*published) {
		t.Fatalf("Current() = (%#v, %v), want %#v", current, err, *published)
	}
	exact, err := reader.Epoch(t.Context(), published.ID)
	if err != nil || !exact.Equal(*published) {
		t.Fatalf("Epoch() = (%#v, %v), want %#v", exact, err, *published)
	}
}

func TestReaderRejectsMissingStoreInvalidIDAndStoredValue(t *testing.T) {
	if _, err := NewReader(nil); err == nil {
		t.Fatal("NewReader(nil) error = nil")
	}
	reader, err := NewReader(&readerStore{})
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	if _, err := reader.Epoch(t.Context(), 0); !errors.Is(err, ErrInvalidEpoch) {
		t.Fatalf("Epoch(0) error = %v, want ErrInvalidEpoch", err)
	}
	invalid := testEpoch(1, testKnowledgeVersions(1), time.Time{})
	reader, err = NewReader(&readerStore{current: invalid})
	if err != nil {
		t.Fatalf("NewReader(invalid store) error = %v", err)
	}
	if _, err := reader.Current(t.Context()); !errors.Is(err, ErrEpochDataIntegrity) {
		t.Fatalf("Current(invalid) error = %v, want ErrEpochDataIntegrity", err)
	}
}

func testEpoch(id ID, versions knowledge.Manifest, publishedAt time.Time) *Epoch {
	return &Epoch{
		ID: id, Knowledge: versions,
		CorporaID: testCorporaID, StructureID: testStructureID,
		PublishedAt: publishedAt,
	}
}

type readerStore struct {
	current *Epoch
}

func (store *readerStore) Current(context.Context) (Epoch, error) {
	if store.current == nil {
		return Epoch{}, ErrEpochNotFound
	}
	return *store.current, nil
}

func (store *readerStore) Load(_ context.Context, id ID) (Epoch, error) {
	if store.current == nil || store.current.ID != id {
		return Epoch{}, ErrEpochNotFound
	}
	return *store.current, nil
}

func (store *readerStore) LoadStructure(_ context.Context, id StructureID) (Epoch, error) {
	if store.current == nil || store.current.StructureID != id {
		return Epoch{}, ErrEpochNotFound
	}
	return *store.current, nil
}
