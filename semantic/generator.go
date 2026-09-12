package semantic

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"
)

type generator struct {
	embedded Embedder
	tokens   tokenCounter
	config   GenerationConfig
}

func (c GenerationConfig) Validate() error {
	if c.BatchSize <= 0 {
		return errors.New("Semantic embedding batch size must be positive")
	}
	if c.BatchMaxTokens <= 0 {
		return errors.New("Semantic embedding batch token limit must be positive")
	}
	if c.MaxConcurrency < 0 {
		return errors.New("Semantic embedding concurrency must not be negative")
	}
	return nil
}

func newGenerator(emb Embedder, tokens tokenCounter, config GenerationConfig) (*generator, error) {
	if emb == nil {
		return nil, errors.New("create Semantic generator: Embedded is required")
	}
	if tokens == nil {
		return nil, errors.New("create Semantic generator: token counter is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = 1
	}
	return &generator{embedded: emb, tokens: tokens, config: config}, nil
}

func (g *generator) generate(ctx context.Context, input []Input) ([]Vector, error) {
	if len(input) == 0 {
		return []Vector{}, nil
	}
	batches, starts, err := g.batches(input)
	if err != nil {
		return nil, err
	}
	responses := make([]EmbeddingBatch, len(batches))
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(g.config.MaxConcurrency)
	for batchIndex := range batches {
		batchIdx := batchIndex
		group.Go(func() error {
			texts := make([]string, len(batches[batchIdx]))
			for index, item := range batches[batchIdx] {
				texts[index] = item.Text
			}
			response, err := g.embedded.Embed(groupContext, texts)
			if err != nil {
				return fmt.Errorf("embed Semantic batch %d: %w", batchIdx, err)
			}
			if len(response.Vectors) != len(texts) {
				return fmt.Errorf(
					"Semantic batch %d returned %d vectors for %d texts",
					batchIdx, len(response.Vectors), len(texts),
				)
			}
			responses[batchIdx] = response
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	result := make([]Vector, len(input))
	dimension := 0
	for batchIndex, response := range responses {
		for offset, values := range response.Vectors {
			normalized, err := NormalizeVector(values)
			if err != nil {
				return nil, fmt.Errorf(
					"validate Semantic batch %d vector %d: %w", batchIndex, offset, err,
				)
			}
			if dimension == 0 {
				dimension = len(normalized)
			} else if len(normalized) != dimension {
				return nil, fmt.Errorf(
					"Semantic vector dimension %d does not match %d", len(normalized), dimension,
				)
			}
			inputIndex := starts[batchIndex] + offset
			result[inputIndex] = Vector{ID: input[inputIndex].ID, Values: normalized}
		}
	}
	return result, nil
}

func (g *generator) batches(input []Input) ([][]Input, []int, error) {
	result := make([][]Input, 0)
	starts := make([]int, 0)
	current := make([]Input, 0, g.config.BatchSize)
	currentTokens := 0
	currentStart := 0
	for index, item := range input {
		count, err := g.tokens.Count(item.Text)
		if err != nil {
			return nil, nil, fmt.Errorf("count Semantic input %q tokens: %w", item.ID, err)
		}
		if count <= 0 {
			return nil, nil, fmt.Errorf("Semantic input %q has no tokens", item.ID)
		}
		if count > g.config.BatchMaxTokens {
			return nil, nil, fmt.Errorf(
				"Semantic input %q has %d tokens; provider limit is %d",
				item.ID, count, g.config.BatchMaxTokens,
			)
		}
		if len(current) > 0 &&
			(len(current) == g.config.BatchSize || currentTokens+count > g.config.BatchMaxTokens) {
			result = append(result, current)
			starts = append(starts, currentStart)
			current = make([]Input, 0, g.config.BatchSize)
			currentTokens = 0
			currentStart = index
		}
		current = append(current, item)
		currentTokens += count
	}
	if len(current) > 0 {
		result = append(result, current)
		starts = append(starts, currentStart)
	}
	return result, starts, nil
}
