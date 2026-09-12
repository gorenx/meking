// Package semantic adapts the semantic vector index to Memory Entity matching.
package semantic

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/vectorindex"
	"github.com/memoria-space/meking/memory"
	semanticbase "github.com/memoria-space/meking/semantic"
)

type Matcher struct {
	vectors  *vectorindex.Service
	semantic *semanticbase.Service
	embedder semanticbase.Embedder
}

func NewMatcher(
	vectors *vectorindex.Service,
	semantic *semanticbase.Service,
	embedder semanticbase.Embedder,
) (*Matcher, error) {
	switch {
	case vectors == nil:
		return nil, errors.New("create Memory Entity Matcher: Entity Vectors are required")
	case semantic == nil:
		return nil, errors.New("create Memory Entity Matcher: Semantic Service is required")
	case embedder == nil:
		return nil, errors.New("create Memory Entity Matcher: Embedder is required")
	default:
		return &Matcher{
			vectors:  vectors,
			semantic: semantic,
			embedder: embedder,
		}, nil
	}
}

func (matcher *Matcher) MatchEntities(
	ctx context.Context,
	query memory.EntityQuery,
) (_ []memory.EntitySimilarity, resultErr error) {
	if matcher == nil {
		return nil, errors.New("Memory Entity Matcher is not configured")
	}
	if len(query.Candidates) == 0 {
		return []memory.EntitySimilarity{}, nil
	}
	if _, err := matcher.vectors.Build(ctx, vectorindex.Target{
		Entities: query.Candidates,
	}); err != nil {
		return nil, fmt.Errorf("prepare current Entity vectors: %w", err)
	}
	namespace, err := vectorindex.Namespace(query.Candidates)
	if err != nil {
		return nil, err
	}
	reader, err := matcher.semantic.Open(ctx, semanticbase.Namespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("open current Entity vectors: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, reader.Close())
	}()
	embedding, err := matcher.embedder.Embed(ctx, []string{query.Text})
	if err != nil {
		return nil, fmt.Errorf("embed Memory query: %w", err)
	}
	if len(embedding.Vectors) != 1 {
		return nil, errors.New("Memory query embedding must contain one vector")
	}
	matches, err := reader.Search(ctx, embedding.Vectors[0], query.Limit, nil)
	if err != nil {
		return nil, fmt.Errorf("search current Entity vectors: %w", err)
	}
	result := make([]memory.EntitySimilarity, len(matches))
	for index, match := range matches {
		reference, err := knowledge.ParseEntityReference(match.ID)
		if err != nil {
			return nil, err
		}
		result[index] = memory.EntitySimilarity{
			Reference: reference,
			Score:     match.Score,
		}
	}
	return result, nil
}
