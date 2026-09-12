package adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	querybase "github.com/memoria-space/meking/query"
)

type tokenCounter interface {
	Count(string) (int, error)
}

// TokenCounter adapts the tokenizer fixed by the Query runtime to Query's
// Corpora-scoped counting contract.
type TokenCounter struct {
	tokens tokenCounter
}

var _ querybase.TokenCounter = (*TokenCounter)(nil)

func NewTokenCounter(tokens tokenCounter) (*TokenCounter, error) {
	if tokens == nil {
		return nil, errors.New("create Query TokenCounter: tokenizer is required")
	}
	return &TokenCounter{tokens: tokens}, nil
}

func (c *TokenCounter) Count(ctx context.Context, corporaID string, text string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if c == nil || c.tokens == nil {
		return 0, errors.New("Query TokenCounter is not configured")
	}
	if strings.TrimSpace(corporaID) == "" {
		return 0, fmt.Errorf("count Query tokens: CorporaID is required")
	}
	count, err := c.tokens.Count(text)
	if err != nil {
		return 0, fmt.Errorf("count Query tokens: %w", err)
	}
	return count, nil
}
