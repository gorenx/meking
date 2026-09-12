package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
)

const MaximumContentBytes int64 = 64 << 20
const streamBufferBytes = 32 << 10

type UploadCommand struct {
	Name      string
	MediaType string
	Content   io.Reader
	// MaximumBytes is an application-selected limit within the fixed Corpus
	// hard limit. Zero uses MaximumContentBytes.
	MaximumBytes int64
}

// StoredContent identifies Document content that is already owned by the
// configured ContentStore and must be validated before metadata is recorded.
type StoredContent struct {
	Name         string
	MediaType    string
	Location     Location
	MaximumBytes int64
}

// PreparedUpload keeps newly published content compensatable until its
// Document metadata has committed.
type PreparedUpload struct {
	LocatedDocument
	staged StagedContent
}

// Confirm records that the owner transaction committed, so the newly
// published content must no longer be removed as compensation.
func (upload PreparedUpload) Confirm() {
	if upload.staged != nil {
		upload.staged.Confirm()
	}
}

func (upload PreparedUpload) Discard() error {
	if upload.staged == nil {
		return nil
	}
	return upload.staged.Discard()
}

type Page struct {
	Offset int
	Limit  int
}

type LocatedDocument struct {
	Document Document
	Location Location
}

type LocatedDocumentPage struct {
	Documents []LocatedDocument
	Next      *Page
}

type SaveResult struct {
	Document        Document
	Location        Location
	DocumentCreated bool
}

type Repository interface {
	Save(ctx context.Context, value Document, location Location) (SaveResult, error)
	Associate(ctx context.Context, ids []ID) ([]LocatedDocument, error)
	Get(ctx context.Context, id ID) (LocatedDocument, error)
	// GetByDigest reads the Project-wide immutable Document used to avoid
	// publishing duplicate content before the current Zone is associated with it.
	GetByDigest(ctx context.Context, digest string) (LocatedDocument, error)
	GetByLocation(ctx context.Context, location Location) (LocatedDocument, error)
	List(ctx context.Context, page Page) (LocatedDocumentPage, error)
}

// Location is a ContentStore-owned file path or object URL.
type Location string

type ContentStore interface {
	Stage(ctx context.Context) (StagedContent, error)
	Open(ctx context.Context, location Location) (io.ReadCloser, error)
}

// StagedContent receives source bytes before their Digest and final Location
// are known. Publish exposes the completed immutable content but keeps it
// compensatable. Confirm records the owner transaction; Discard removes an
// unconfirmed stage and any final content created by that stage.
type StagedContent interface {
	io.Writer
	Publish(ctx context.Context, value Document) (Location, error)
	// Confirm records that the published content is durably referenced by
	// Document metadata and disables the compensating removal in Discard.
	Confirm()
	Discard() error
}

// Manager is the application boundary for immutable source documents.
type Manager struct {
	store   Repository
	content ContentStore
}

func NewManager(store Repository, content ContentStore) (*Manager, error) {
	if store == nil {
		return nil, errors.New("create document Manager: store is required")
	}
	if content == nil {
		return nil, errors.New("create document Manager: content store is required")
	}
	return &Manager{store: store, content: content}, nil
}

// PrepareUpload validates source bytes and publishes immutable content without
// opening a metadata transaction. The caller must Confirm the result only after
// its owner transaction commits, or Discard it on failure.
func (m *Manager) PrepareUpload(
	ctx context.Context,
	command UploadCommand,
) (result PreparedUpload, resultErr error) {
	if err := ctx.Err(); err != nil {
		return PreparedUpload{}, err
	}
	if command.Content == nil {
		return PreparedUpload{}, fmt.Errorf("%w: content is required", ErrInvalid)
	}
	staged, err := m.content.Stage(ctx)
	if err != nil {
		return PreparedUpload{}, fmt.Errorf("stage source document content: %w", err)
	}
	returned := false
	defer func() {
		if !returned {
			resultErr = errors.Join(resultErr, staged.Discard())
		}
	}()
	value, err := m.create(
		ctx,
		command.Name,
		command.MediaType,
		command.Content,
		command.MaximumBytes,
		staged,
	)
	if err != nil {
		return PreparedUpload{}, err
	}
	existing, err := m.store.GetByDigest(ctx, value.Digest)
	if err == nil {
		if existing.Document.Size != value.Size {
			return PreparedUpload{}, fmt.Errorf("%w: Digest identifies different content sizes", ErrStorageIntegrity)
		}
		if err := staged.Discard(); err != nil {
			return PreparedUpload{}, err
		}
		returned = true
		return PreparedUpload{LocatedDocument: existing}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return PreparedUpload{}, err
	}
	location, err := staged.Publish(ctx, value)
	if err != nil {
		return PreparedUpload{}, fmt.Errorf("store source document content: %w", err)
	}
	if location == "" {
		return PreparedUpload{}, fmt.Errorf("%w: content store returned an empty location", ErrStorageIntegrity)
	}
	returned = true
	return PreparedUpload{
		LocatedDocument: LocatedDocument{Document: value, Location: location},
		staged:          staged,
	}, nil
}

// Record persists the metadata of content already available from ContentStore.
func (m *Manager) Record(ctx context.Context, value LocatedDocument) (SaveResult, error) {
	if _, err := Restore(value.Document); err != nil {
		return SaveResult{}, err
	}
	if value.Location == "" {
		return SaveResult{}, fmt.Errorf("%w: content location is required", ErrInvalid)
	}
	return m.store.Save(ctx, value.Document, value.Location)
}

func (m *Manager) Associate(ctx context.Context, ids []ID) ([]LocatedDocument, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: Document IDs are required", ErrInvalid)
	}
	normalized := make([]ID, len(ids))
	copy(normalized, ids)
	for _, id := range normalized {
		if err := ValidateID(id); err != nil {
			return nil, err
		}
	}
	return m.store.Associate(ctx, normalized)
}

// PrepareExisting validates content already owned by ContentStore without
// changing Document metadata.
func (m *Manager) PrepareExisting(ctx context.Context, content StoredContent) (LocatedDocument, error) {
	if content.Location == "" {
		return LocatedDocument{}, fmt.Errorf("%w: content location is required", ErrInvalid)
	}
	stream, err := m.content.Open(ctx, content.Location)
	if err != nil {
		return LocatedDocument{}, err
	}
	defer stream.Close()
	value, err := m.create(
		ctx,
		content.Name,
		content.MediaType,
		stream,
		content.MaximumBytes,
		io.Discard,
	)
	if err != nil {
		return LocatedDocument{}, err
	}
	return LocatedDocument{Document: value, Location: content.Location}, nil
}

func (m *Manager) Get(ctx context.Context, id ID) (Document, error) {
	v, err := m.store.Get(ctx, id)
	if err != nil {
		return Document{}, err
	}
	return v.Document, err
}

// Located returns Document metadata together with its ContentStore location.
// Callers use it when an existing stored object must be recorded in another scope.
func (m *Manager) Located(ctx context.Context, id ID) (LocatedDocument, error) {
	return m.store.Get(ctx, id)
}

func (m *Manager) Open(ctx context.Context, id ID) (io.ReadCloser, error) {
	located, err := m.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if located.Location == "" {
		return nil, fmt.Errorf("%w: Document %q has no content Location", ErrStorageIntegrity, id)
	}
	stream, err := m.content.Open(ctx, located.Location)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", located.Location, err)
	}
	return &verifiedContentReader{
		ctx: ctx, stream: stream, value: located.Document, digest: sha256.New(),
	}, nil
}

func (m *Manager) ListLocated(ctx context.Context, page Page) (LocatedDocumentPage, error) {
	if page.Offset < 0 || page.Limit <= 0 || page.Limit > 1000 {
		return LocatedDocumentPage{}, fmt.Errorf("%w: page is invalid", ErrInvalid)
	}
	return m.store.List(ctx, page)
}

func (m *Manager) LocatedByLocation(ctx context.Context, location Location) (LocatedDocument, error) {
	if location == "" {
		return LocatedDocument{}, fmt.Errorf("%w: content location is required", ErrInvalid)
	}
	return m.store.GetByLocation(ctx, location)
}

func (m *Manager) create(
	ctx context.Context,
	name string,
	mediaType string,
	reader io.Reader,
	maximumBytes int64,
	destination io.Writer,
) (Document, error) {
	if maximumBytes == 0 {
		maximumBytes = MaximumContentBytes
	}
	if maximumBytes < 0 || maximumBytes > MaximumContentBytes {
		return Document{}, fmt.Errorf(
			"%w: maximum content bytes must be between 1 and %d",
			ErrInvalid,
			MaximumContentBytes,
		)
	}
	limited := &io.LimitedReader{R: &contextReader{ctx: ctx, reader: reader}, N: maximumBytes + 1}
	digest := sha256.New()
	size, err := io.CopyBuffer(io.MultiWriter(destination, digest), limited, make([]byte, streamBufferBytes))
	if err != nil {
		return Document{}, fmt.Errorf("stream source document: %w", err)
	}
	if size > maximumBytes {
		return Document{}, fmt.Errorf("%w: maximum is %d bytes", ErrContentTooLarge, maximumBytes)
	}
	if size == 0 {
		return Document{}, fmt.Errorf("%w: content is required", ErrInvalid)
	}
	return newDocument(name, mediaType, size, hex.EncodeToString(digest.Sum(nil)))
}

type verifiedContentReader struct {
	ctx      context.Context
	stream   io.ReadCloser
	value    Document
	digest   hash.Hash
	size     int64
	finished bool
}

func (r *verifiedContentReader) Read(buffer []byte) (int, error) {
	if r.finished {
		return 0, io.EOF
	}
	if err := r.ctx.Err(); err != nil {
		r.finished = true
		return 0, err
	}
	read, readErr := r.stream.Read(buffer)
	if read > 0 {
		_, _ = r.digest.Write(buffer[:read])
		r.size += int64(read)
		if r.size > r.value.Size {
			r.finished = true
			return read, fmt.Errorf("%w: size exceeds metadata", ErrStorageIntegrity)
		}
	}
	if readErr == nil {
		return read, nil
	}
	r.finished = true
	if !errors.Is(readErr, io.EOF) {
		return read, readErr
	}
	if err := verifyContent(r.value, r.size, hex.EncodeToString(r.digest.Sum(nil))); err != nil {
		return read, err
	}
	return read, io.EOF
}

func (r *verifiedContentReader) Close() error {
	var verifyErr error
	if !r.finished {
		_, verifyErr = io.CopyBuffer(io.Discard, r, make([]byte, streamBufferBytes))
	}
	return errors.Join(verifyErr, r.stream.Close())
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
