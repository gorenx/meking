package text

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/internal/uuid"
)

type DocumentReader interface {
	Get(context.Context, document.ID) (document.Document, error)
	Open(context.Context, document.ID) (io.ReadCloser, error)
}

type Store interface {
	Save(context.Context, Text) (Text, error)
	Get(context.Context, ID) (Text, error)
	Find(context.Context, document.ID, string, string) (Text, error)
	List(context.Context, Page) (TextPage, error)
}

type Builder struct {
	documents  DocumentReader
	extractors ExtractorResolver
	normalizer Normalizer
	store      Store
}

func NewBuilder(
	documents DocumentReader,
	extractors ExtractorResolver,
	normalizer Normalizer,
	store Store,
) (*Builder, error) {
	switch {
	case documents == nil:
		return nil, errors.New("create Text Builder: DocumentReader is required")
	case extractors == nil:
		return nil, errors.New("create Text Builder: ExtractorResolver is required")
	case normalizer == nil:
		return nil, errors.New("create Text Builder: Normalizer is required")
	case store == nil:
		return nil, errors.New("create Text Builder: Store is required")
	default:
		return &Builder{documents: documents, extractors: extractors, normalizer: normalizer, store: store}, nil
	}
}

// Prepare extracts and normalizes a Document without opening a persistence transaction.
func (b *Builder) Prepare(ctx context.Context, documentID document.ID) (Text, error) {
	value, err := b.documents.Get(ctx, documentID)
	if err != nil {
		return Text{}, err
	}
	extractor, _, err := b.extractors.Resolve(value.Name, value.MediaType)
	if err != nil {
		return Text{}, err
	}
	existing, err := b.store.Find(
		ctx,
		documentID,
		extractor.Profile(),
		b.normalizer.Profile(),
	)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Text{}, err
	}
	stream, err := b.documents.Open(ctx, documentID)
	if err != nil {
		return Text{}, err
	}
	extracted, extractErr := extractor.Extract(ctx, value, stream)
	closeErr := stream.Close()
	if extractErr != nil || closeErr != nil {
		return Text{}, fmt.Errorf("extract Document %q: %w", documentID, errors.Join(extractErr, closeErr))
	}
	body, err := b.normalizer.Normalize(ctx, extracted.Body)
	if err != nil {
		return Text{}, fmt.Errorf("normalize Document %q: %w", documentID, err)
	}
	id, err := uuid.NewV4()
	if err != nil {
		return Text{}, fmt.Errorf("create Text ID: %w", err)
	}
	result := Text{
		ID:                   ID(id),
		DocumentID:           documentID,
		Title:                strings.TrimSpace(extracted.Title),
		Body:                 body,
		Format:               extracted.Format,
		ExtractionProfile:    strings.TrimSpace(extractor.Profile()),
		NormalizationProfile: strings.TrimSpace(b.normalizer.Profile()),
		Warnings:             append([]string(nil), extracted.Warnings...),
	}
	if err = Validate(result); err != nil {
		return Text{}, err
	}
	return result, nil
}

// Save persists a fully prepared immutable Text through the caller's transaction.
func (b *Builder) Save(ctx context.Context, result Text) (Text, error) {
	return b.store.Save(ctx, result)
}
