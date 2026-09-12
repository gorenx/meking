package adapter

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/memoria-space/meking/knowledge"
	knowledgevector "github.com/memoria-space/meking/knowledge/vectorindex"
	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
	"github.com/memoria-space/meking/semantic"
)

type namespaceOpener interface {
	Open(ctx context.Context, namespace semantic.Namespace) (semantic.NamespaceReader, error)
}

type EntityVectorStore struct {
	store namespaceOpener
}

var _ querylocal.EntityVectorStore = (*EntityVectorStore)(nil)

func NewEntityVectorStore(store namespaceOpener) (*EntityVectorStore, error) {
	if store == nil {
		return nil, errors.New("create Local EntityVectorStore: Semantic store is required")
	}
	return &EntityVectorStore{store: store}, nil
}

func (s *EntityVectorStore) Open(
	ctx context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
) (querylocal.EntityVectorReader, error) {
	if s == nil || s.store == nil {
		return nil, querybase.NewInternalFailure(
			errors.New("Local EntityVectorStore is not configured"),
		)
	}
	namespaceID, err := knowledgevector.Namespace(entities)
	if err != nil {
		return nil, querybase.NewPublicationIncompleteFailure(err)
	}
	namespace := semantic.Namespace(namespaceID)
	reader, err := s.store.Open(ctx, namespace)
	if err != nil {
		return nil, entityVectorOpenFailure(err)
	}
	info, err := reader.Info(ctx)
	if err != nil {
		return nil, closeEntityNamespace(reader, entityVectorOpenFailure(err))
	}
	if info.Namespace != namespace {
		return nil, closeEntityNamespace(reader, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
			"Local Entity vector reader opened Namespace %q; expected %q",
			info.Namespace, namespace,
		)))
	}
	return &entityVectorReader{reader: reader, info: info}, nil
}

type entityVectorReader struct {
	reader semantic.NamespaceReader
	info   semantic.Info
	mu     sync.RWMutex

	closeOnce sync.Once
	closed    bool
	closeErr  error
}

func (r *entityVectorReader) Model() string {
	if r == nil {
		return ""
	}
	return r.info.Model
}

func (r *entityVectorReader) Dimension() int {
	if r == nil {
		return -1
	}
	return r.info.Dimension
}

func (r *entityVectorReader) Search(
	ctx context.Context,
	vector []float64,
	limit int,
) ([]querylocal.EntityMatch, error) {
	if r == nil || r.reader == nil {
		return nil, querybase.NewInternalFailure(
			errors.New("Local EntityVectorReader is not configured"),
		)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, querybase.NewInternalFailure(
			errors.New("Local EntityVectorReader is closed"),
		)
	}
	matches, err := r.reader.Search(ctx, vector, limit, nil)
	if err != nil {
		return nil, entityVectorReadFailure(err)
	}
	result := make([]querylocal.EntityMatch, len(matches))
	for index, match := range matches {
		reference, err := knowledge.ParseEntityReference(match.ID)
		if err != nil {
			return nil, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
				"parse Local Entity vector ID %q: %w", match.ID, err,
			))
		}
		result[index] = querylocal.EntityMatch{
			ID: string(reference.ID), Version: uint64(reference.Version), Score: match.Score,
		}
	}
	return result, nil
}

func (r *entityVectorReader) Close() error {
	if r == nil || r.reader == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.closed = true
		r.closeErr = r.reader.Close()
	})
	return r.closeErr
}

func entityVectorOpenFailure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewPublicationIncompleteFailure(err)
}

func entityVectorReadFailure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewInternalFailure(err)
}

func closeEntityNamespace(reader semantic.NamespaceReader, resultErr error) error {
	if closeErr := reader.Close(); closeErr != nil {
		return errors.Join(
			resultErr,
			querybase.NewInternalFailure(fmt.Errorf("close Local Entity vector Namespace: %w", closeErr)),
		)
	}
	return resultErr
}
