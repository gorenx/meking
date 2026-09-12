package adapter

import (
	"context"
	"errors"

	querybase "github.com/memoria-space/meking/query"
	querydrift "github.com/memoria-space/meking/query/drift"
	querylocal "github.com/memoria-space/meking/query/local"
)

// LocalSessionOpener maps DRIFT's fixed-Epoch port to Local's public Session
// API without exposing Local construction details to the DRIFT aggregate.
type LocalSessionOpener struct {
	searcher *querylocal.Searcher
}

var _ querydrift.LocalSessionOpener = (*LocalSessionOpener)(nil)

func NewLocalSessionOpener(searcher *querylocal.Searcher) (*LocalSessionOpener, error) {
	if searcher == nil {
		return nil, errors.New("create DRIFT LocalSessionOpener: Local Searcher is required")
	}
	return &LocalSessionOpener{searcher: searcher}, nil
}

func (o *LocalSessionOpener) Open(
	ctx context.Context,
	selected querybase.Epoch,
) (querydrift.LocalSession, error) {
	if o == nil || o.searcher == nil {
		return nil, querybase.NewInternalFailure(
			errors.New("DRIFT LocalSessionOpener is not configured"),
		)
	}
	return o.searcher.OpenAt(ctx, selected)
}
