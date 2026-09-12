package document

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"testing"
)

func TestManagerPrepareUploadHonorsApplicationContentLimit(t *testing.T) {
	repository := newMemoryRepository()
	content := newMemoryContentStore()
	manager, err := NewManager(repository, content)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	_, err = manager.PrepareUpload(t.Context(), UploadCommand{
		Name: "source.pdf", MediaType: "application/pdf",
		Content: bytes.NewBufferString("12345"), MaximumBytes: 4,
	})
	if !errors.Is(err, ErrContentTooLarge) {
		t.Fatalf("PrepareUpload() error = %v, want ErrContentTooLarge", err)
	}
	if len(repository.documents) != 0 || content.putCalls != 0 {
		t.Fatalf("rejected upload persisted documents=%d puts=%d", len(repository.documents), content.putCalls)
	}
}

func TestManagerPrepareUploadStreamsToContentStore(t *testing.T) {
	repository := newMemoryRepository()
	content := newMemoryContentStore()
	manager, err := NewManager(repository, content)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("a"), streamBufferBytes*3)
	prepared, err := manager.PrepareUpload(t.Context(), UploadCommand{
		Name: "source.txt", MediaType: "text/plain", Content: bytes.NewReader(payload),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Discard()
	if content.maximumWrite > streamBufferBytes {
		t.Fatalf("maximum staged write = %d, want <= %d", content.maximumWrite, streamBufferBytes)
	}
}

func TestManagerPrepareAndRecordIsIdempotentByDigest(t *testing.T) {
	repository := newMemoryRepository()
	content := newMemoryContentStore()
	manager, err := NewManager(repository, content)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	first, err := prepareAndRecord(t.Context(), manager, UploadCommand{
		Name: "source.txt", MediaType: "text/plain",
		Content: bytes.NewBufferString("knowledge"),
	})
	if err != nil {
		t.Fatalf("first Prepare and Record: %v", err)
	}
	second, err := prepareAndRecord(t.Context(), manager, UploadCommand{
		Name: "renamed.md", MediaType: "text/markdown",
		Content: bytes.NewBufferString("knowledge"),
	})
	if err != nil {
		t.Fatalf("second Prepare and Record: %v", err)
	}
	if first.Document.ID != second.Document.ID || !first.DocumentCreated ||
		first.Location == "" || second.DocumentCreated || second.Location != first.Location ||
		len(repository.documents) != 1 || content.putCalls != 1 {
		t.Fatalf("first=%#v second=%#v documents=%d puts=%d", first, second, len(repository.documents), content.putCalls)
	}
}

func TestManagerRecordPreparedExistingDoesNotReplaceDigestDocumentLocation(t *testing.T) {
	repository := newMemoryRepository()
	content := newMemoryContentStore()
	manager, err := NewManager(repository, content)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	first, err := prepareAndRecord(t.Context(), manager, UploadCommand{
		Name: "source.txt", MediaType: "text/plain",
		Content: bytes.NewBufferString("knowledge"),
	})
	if err != nil {
		t.Fatalf("Prepare and Record: %v", err)
	}
	location := Location("memory://existing/renamed.txt")
	content.values[location] = []byte("knowledge")
	prepared, err := manager.PrepareExisting(t.Context(), StoredContent{
		Name: "renamed.txt", MediaType: "text/plain", Location: location,
	})
	if err != nil {
		t.Fatalf("PrepareExisting: %v", err)
	}
	second, err := manager.Record(t.Context(), prepared)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if second.Document.ID != first.Document.ID || second.DocumentCreated ||
		second.Location != first.Location || len(repository.byLocation) != 1 || content.putCalls != 1 {
		t.Fatalf("first=%#v second=%#v locations=%v puts=%d", first, second, repository.byLocation, content.putCalls)
	}
}

func TestManagerPrepareExistingDoesNotPersistMetadata(t *testing.T) {
	repository := newMemoryRepository()
	content := newMemoryContentStore()
	manager, err := NewManager(repository, content)
	if err != nil {
		t.Fatal(err)
	}
	location := Location("memory://existing/source.txt")
	content.values[location] = []byte("knowledge")
	prepared, err := manager.PrepareExisting(t.Context(), StoredContent{
		Name: "source.txt", MediaType: "text/plain", Location: location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Location != location || prepared.Document.Digest == "" || len(repository.documents) != 0 {
		t.Fatalf("prepared=%#v documents=%d", prepared, len(repository.documents))
	}
}

func prepareAndRecord(ctx context.Context, manager *Manager, command UploadCommand) (SaveResult, error) {
	prepared, err := manager.PrepareUpload(ctx, command)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := manager.Record(ctx, prepared.LocatedDocument)
	if err != nil {
		return SaveResult{}, errors.Join(err, prepared.Discard())
	}
	prepared.Confirm()
	return result, nil
}

type memoryRepository struct {
	documents  map[ID]Document
	byDigest   map[string]ID
	byLocation map[Location]ID
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		documents: make(map[ID]Document), byDigest: make(map[string]ID),
		byLocation: make(map[Location]ID),
	}
}

func (r *memoryRepository) Save(_ context.Context, value Document, location Location) (SaveResult, error) {
	if id, exists := r.byDigest[value.Digest]; exists {
		stored := r.documents[id]
		for existingLocation, documentID := range r.byLocation {
			if documentID == id {
				return SaveResult{Document: stored, Location: existingLocation}, nil
			}
		}
		return SaveResult{}, ErrStorageIntegrity
	}
	if _, exists := r.byLocation[location]; exists {
		return SaveResult{}, ErrContentConflict
	}
	r.documents[value.ID] = value
	r.byDigest[value.Digest] = value.ID
	r.byLocation[location] = value.ID
	return SaveResult{Document: value, Location: location, DocumentCreated: true}, nil
}

func (r *memoryRepository) Associate(
	ctx context.Context,
	ids []ID,
) ([]LocatedDocument, error) {
	result := make([]LocatedDocument, len(ids))
	for index, id := range ids {
		value, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		result[index] = value
	}
	return result, nil
}

func (r *memoryRepository) Get(_ context.Context, id ID) (LocatedDocument, error) {
	value, exists := r.documents[id]
	if !exists {
		return LocatedDocument{}, ErrNotFound
	}
	for location, documentID := range r.byLocation {
		if documentID == id {
			return LocatedDocument{Document: value, Location: location}, nil
		}
	}
	return LocatedDocument{}, ErrStorageIntegrity
}

func (r *memoryRepository) GetByDigest(ctx context.Context, digest string) (LocatedDocument, error) {
	id, exists := r.byDigest[digest]
	if !exists {
		return LocatedDocument{}, ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *memoryRepository) GetByLocation(ctx context.Context, location Location) (LocatedDocument, error) {
	id, exists := r.byLocation[location]
	if !exists {
		return LocatedDocument{}, ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *memoryRepository) List(ctx context.Context, page Page) (LocatedDocumentPage, error) {
	values := make([]LocatedDocument, 0, len(r.documents))
	for id := range r.documents {
		value, err := r.Get(ctx, id)
		if err != nil {
			return LocatedDocumentPage{}, err
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Document.ID < values[j].Document.ID })
	if page.Offset >= len(values) {
		return LocatedDocumentPage{}, nil
	}
	end := min(page.Offset+page.Limit, len(values))
	result := LocatedDocumentPage{Documents: values[page.Offset:end]}
	if end < len(values) {
		result.Next = &Page{Offset: end, Limit: page.Limit}
	}
	return result, nil
}

type memoryContentStore struct {
	values       map[Location][]byte
	putCalls     int
	maximumWrite int
}

func newMemoryContentStore() *memoryContentStore {
	return &memoryContentStore{values: make(map[Location][]byte)}
}

func (s *memoryContentStore) Stage(context.Context) (StagedContent, error) {
	return &memoryStagedContent{store: s}, nil
}

func (s *memoryContentStore) Open(_ context.Context, location Location) (io.ReadCloser, error) {
	data, exists := s.values[location]
	if !exists {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

type memoryStagedContent struct {
	store     *memoryContentStore
	content   bytes.Buffer
	committed bool
	retained  bool
	location  Location
}

func (s *memoryStagedContent) Write(content []byte) (int, error) {
	s.store.maximumWrite = max(s.store.maximumWrite, len(content))
	return s.content.Write(content)
}

func (s *memoryStagedContent) Publish(_ context.Context, value Document) (Location, error) {
	s.store.putCalls++
	location := Location("memory://documents/" + value.Digest)
	s.store.values[location] = append([]byte(nil), s.content.Bytes()...)
	s.committed = true
	s.location = location
	return location, nil
}

func (s *memoryStagedContent) Confirm() { s.retained = true }

func (s *memoryStagedContent) Discard() error {
	if s.committed && !s.retained {
		delete(s.store.values, s.location)
	}
	s.content.Reset()
	return nil
}
