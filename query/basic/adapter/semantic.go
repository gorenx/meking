package adapter

import (
	"context"
	"errors"

	textunitvector "github.com/memoria-space/meking/corpus/textunits/vectorindex"
	querybase "github.com/memoria-space/meking/query"
	querybasic "github.com/memoria-space/meking/query/basic"
	"github.com/memoria-space/meking/semantic"
)

type VectorStore struct {
	store namespaceOpener
}

type namespaceOpener interface {
	Open(ctx context.Context, namespace semantic.Namespace) (semantic.NamespaceReader, error)
}

var _ querybasic.VectorStore = (*VectorStore)(nil)

func NewVectorStore(store namespaceOpener) (*VectorStore, error) {
	if store == nil {
		return nil, errors.New("create Basic VectorStore: Semantic store is required")
	}
	return &VectorStore{store: store}, nil
}

func (s *VectorStore) OpenTextUnits(
	ctx context.Context,
	corporaID string,
) (querybasic.TextUnitVectorReader, error) {
	if s == nil || s.store == nil {
		return nil, querybase.NewInternalFailure(errors.New("Basic VectorStore is not configured"))
	}
	name, err := textunitvector.Namespace(corporaID)
	if err != nil {
		return nil, querybase.NewPublicationIncompleteFailure(err)
	}
	reader, err := s.store.Open(ctx, semantic.Namespace(name))
	if err != nil {
		return nil, openFailure(err)
	}
	info, err := reader.Info(ctx)
	if err != nil {
		return nil, errors.Join(openFailure(err), reader.Close())
	}
	return &textUnitVectorReader{reader: reader, info: info}, nil
}

type textUnitVectorReader struct {
	reader semantic.NamespaceReader
	info   semantic.Info
}

func (c *textUnitVectorReader) CorporaID() string { return string(c.info.Namespace) }
func (c *textUnitVectorReader) Model() string     { return c.info.Model }
func (c *textUnitVectorReader) Dimension() int    { return c.info.Dimension }

func (c *textUnitVectorReader) Search(
	ctx context.Context,
	vector []float64,
	limit int,
) ([]querybasic.TextUnitMatch, error) {
	if c == nil || c.reader == nil {
		return nil, querybase.NewInternalFailure(errors.New("Basic TextUnitVectorReader is not configured"))
	}
	matches, err := c.reader.Search(ctx, vector, limit, nil)
	if err != nil {
		return nil, readFailure(err)
	}
	result := make([]querybasic.TextUnitMatch, len(matches))
	for index, match := range matches {
		result[index] = querybasic.TextUnitMatch{TextUnitID: match.ID, Score: match.Score}
	}
	return result, nil
}

func (c *textUnitVectorReader) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}

func openFailure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	return querybase.NewPublicationIncompleteFailure(err)
}

func readFailure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	return querybase.NewInternalFailure(err)
}
