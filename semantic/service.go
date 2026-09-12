package semantic

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type vectorCache interface {
	Lookup(ctx context.Context, ids []string) ([]Vector, error)
}

// Service adds complete-text vectors to caller-owned Namespaces. It never
// interprets vector IDs, Namespace names, or source pagination.
type Service struct {
	store NamespaceStore
	cache vectorCache
	gen   *generator
}

func NewService(
	embedded Embedder,
	tokens tokenCounter,
	store NamespaceStore,
	config GenerationConfig,
) (*Service, error) {
	if store == nil {
		return nil, errors.New("create Semantic Service: NamespaceStore is required")
	}
	cache, ok := store.(vectorCache)
	if !ok {
		return nil, errors.New("create Semantic Service: vector cache is required")
	}
	gen, err := newGenerator(embedded, tokens, config)
	if err != nil {
		return nil, err
	}
	return &Service{store: store, cache: cache, gen: gen}, nil
}

func (s *Service) Open(ctx context.Context, namespace Namespace) (NamespaceReader, error) {
	return s.store.Open(ctx, namespace)
}

func (s *Service) Delete(ctx context.Context, namespace Namespace) error {
	return s.store.Delete(ctx, namespace)
}

func (s *Service) Add(
	ctx context.Context,
	namespace Namespace,
	input []Input,
) error {
	if err := ValidateNamespace(namespace); err != nil {
		return err
	}
	if len(input) == 0 {
		return s.store.Add(ctx, namespace, nil)
	}
	requested := make([]string, len(input))
	pending := make(map[string]struct{}, len(input))
	for index, item := range input {
		if err := ValidateID(item.ID, "vector ID"); err != nil {
			return fmt.Errorf("Semantic input %d: %w", index, err)
		}
		if strings.TrimSpace(item.Text) == "" {
			return fmt.Errorf("Semantic input %q text is required", item.ID)
		}
		if _, duplicate := pending[item.ID]; duplicate {
			return fmt.Errorf("Semantic input vector ID %q is duplicated", item.ID)
		}
		pending[item.ID] = struct{}{}
		requested[index] = item.ID
	}
	cached, err := s.cache.Lookup(ctx, requested)
	if err != nil {
		return fmt.Errorf("query Semantic vector cache: %w", err)
	}
	cachedIDs := make(map[string]struct{}, len(cached))
	for _, vector := range cached {
		if _, req := pending[vector.ID]; !req {
			return fmt.Errorf("Semantic cache returned unrequested vector ID %q", vector.ID)
		}
		cachedIDs[vector.ID] = struct{}{}
	}
	missing := make([]Input, 0, len(input)-len(cachedIDs))
	for _, item := range input {
		if _, exists := cachedIDs[item.ID]; !exists {
			missing = append(missing, item)
		}
	}
	generated, err := s.gen.generate(ctx, missing)
	if err != nil {
		return err
	}
	vectors := make([]Vector, 0, len(cached)+len(generated))
	vectors = append(vectors, cached...)
	vectors = append(vectors, generated...)
	return s.store.Add(ctx, namespace, vectors)
}
