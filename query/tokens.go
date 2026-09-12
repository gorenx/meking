package query

import "context"

// TokenCounter counts text with the tokenizer recorded by one immutable
// Corpora. Query aggregates supply the already-fixed CorporaID and never
// use this contract to resolve Current.
type TokenCounter interface {
	Count(ctx context.Context, corporaID string, text string) (int, error)
}
