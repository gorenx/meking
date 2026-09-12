package basic

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultTopK is the number of TextUnit vector records requested before
	// duplicate source identities are folded.
	DefaultTopK = 10
	// DefaultContextTokens is the hard token budget for the rendered Sources table.
	DefaultContextTokens = 12000
	// DefaultResponseType requests the established multi-paragraph answer shape.
	DefaultResponseType = "Multiple Paragraphs"
)

// Config owns Basic Search retrieval, context, and default presentation policy.
// It is validated when a Searcher is constructed and is immutable afterwards.
type Config struct {
	// TopK is the positive maximum number of physical vector matches requested.
	TopK int
	// MaxContextTokens is the positive hard budget for the escaped Sources table.
	MaxContextTokens int
	// Delimiter is the single non-quote, non-newline rune used by the Sources table.
	Delimiter rune
	// ResponseType is the non-empty default inserted into the answer prompt when
	// a request does not supply an override.
	ResponseType string
}

// DefaultConfig returns Basic Search's built-in bounded retrieval policy.
func DefaultConfig() Config {
	return Config{
		TopK:             DefaultTopK,
		MaxContextTokens: DefaultContextTokens,
		Delimiter:        '|',
		ResponseType:     DefaultResponseType,
	}
}

// Validate rejects policies that cannot produce a deterministic bounded query.
func (c Config) Validate() error {
	if c.TopK <= 0 {
		return errors.New("Basic Search TextUnit limit must be positive")
	}
	if c.MaxContextTokens <= 0 {
		return errors.New("Basic Search context token budget must be positive")
	}
	if c.Delimiter == 0 || c.Delimiter == '"' || c.Delimiter == '\r' ||
		c.Delimiter == '\n' || c.Delimiter == utf8.RuneError {
		return errors.New("Basic Search delimiter must be one non-quote line rune")
	}
	if strings.TrimSpace(c.ResponseType) == "" || c.ResponseType != strings.TrimSpace(c.ResponseType) {
		return errors.New("Basic Search default response type is required without surrounding whitespace")
	}
	return nil
}
