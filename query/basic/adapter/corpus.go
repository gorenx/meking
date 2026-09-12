package adapter

import (
	"context"
	"errors"
	"fmt"
	"github.com/memoria-space/meking/corpus/textunits"
	"sync"

	"github.com/memoria-space/meking/corpus"
	querybase "github.com/memoria-space/meking/query"
	querybasic "github.com/memoria-space/meking/query/basic"
)

// CorpusCatalog is the provider-owned exact Chunking capability needed by Basic.
// Epoch already supplies the selected Corpora identity.
type CorpusCatalog interface {
	CorporaChunking(ctx context.Context, id corpus.CorporaID) (textunits.Chunking, error)
}

// CorpusReader maps exact Corpus tokenization into Basic. Tokenizers are cached
// by immutable Corpora identity for concurrent queries.
type CorpusReader struct {
	// catalog is the Corpus fact source for current identity and exact Chunking.
	catalog CorpusCatalog
	// mutex protects tokenizers while one adapter serves concurrent queries.
	mutex sync.RWMutex
	// tokenizers contains one immutable tokenizer per already-used Corpora ID.
	tokenizers map[corpus.CorporaID]*textunits.TiktokenTokenizer
}

var (
	_ querybasic.TokenCounter = (*CorpusReader)(nil)
)

// NewCorpusReader creates the Basic Corpus adapter without resolving Current.
func NewCorpusReader(catalog CorpusCatalog) (*CorpusReader, error) {
	if catalog == nil {
		return nil, errors.New("create Basic CorpusReader: Corpus Catalog is required")
	}
	return &CorpusReader{
		catalog:    catalog,
		tokenizers: make(map[corpus.CorporaID]*textunits.TiktokenTokenizer),
	}, nil
}

// Count resolves the exact Corpora's immutable EncodingModel once and reuses
// its tokenizer. It never reads current Project settings.
func (r *CorpusReader) Count(
	ctx context.Context,
	corporaID string,
	text string,
) (int, error) {
	if r == nil || r.catalog == nil {
		return 0, querybase.NewInternalFailure(errors.New("Basic CorpusReader is not configured"))
	}
	id := corpus.CorporaID(corporaID)
	r.mutex.RLock()
	tokenizer := r.tokenizers[id]
	r.mutex.RUnlock()
	if tokenizer != nil {
		count, err := tokenizer.Count(text)
		if err != nil {
			return 0, querybase.NewInternalFailure(err)
		}
		return count, nil
	}
	chunking, err := r.catalog.CorporaChunking(ctx, id)
	if err != nil {
		if errors.Is(err, corpus.ErrCorporaNotFound) ||
			errors.Is(err, corpus.ErrCorpusDataIntegrity) ||
			errors.Is(err, corpus.ErrInvalidCorpus) {
			return 0, querybase.NewPublicationIncompleteFailure(err)
		}
		return 0, corpusFailure(err)
	}
	tokenizer, err = textunits.NewTiktokenTokenizer(chunking.EncodingModel)
	if err != nil {
		return 0, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
			"create tokenizer for Corpora %q: %w",
			corporaID,
			err,
		))
	}
	r.mutex.Lock()
	if cached := r.tokenizers[id]; cached != nil {
		tokenizer = cached
	} else {
		r.tokenizers[id] = tokenizer
	}
	r.mutex.Unlock()
	count, err := tokenizer.Count(text)
	if err != nil {
		return 0, querybase.NewInternalFailure(err)
	}
	return count, nil
}

func corpusFailure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	return querybase.NewInternalFailure(err)
}
