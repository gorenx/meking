package textunits

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	tiktoken "github.com/tiktoken-go/tokenizer"
)

const (
	// DefaultEncodingModel is the default OpenAI token vocabulary for chunking.
	DefaultEncodingModel = "o200k_base"
)

var (
	// ErrDisallowedSpecialToken matches tiktoken's default refusal to encode special tokens as ordinary text.
	ErrDisallowedSpecialToken = errors.New("text contains a disallowed special token")
)

// TokenID is an identifier in a tokenizer vocabulary.
type TokenID uint

// Token preserves one tokenizer piece exactly as it occurs in source text.
type Token struct {
	ID   TokenID
	Text string
}

// TiktokenTokenizer adapts an embedded tiktoken vocabulary to corpus token operations.
type TiktokenTokenizer struct {
	encodingName      string
	codec             tiktoken.Codec
	disallowedSpecial []string
}

// NewTiktokenTokenizer creates a tokenizer for a supported tiktoken encoding.
func NewTiktokenTokenizer(encodingName string) (*TiktokenTokenizer, error) {
	encoding, special, err := resolveTiktokenEncoding(strings.TrimSpace(encodingName))
	if err != nil {
		return nil, err
	}
	codec, err := tiktoken.Get(encoding)
	if err != nil {
		return nil, fmt.Errorf("create tokenizer for encoding %q: %w", encodingName, err)
	}
	return &TiktokenTokenizer{
		encodingName:      string(encoding),
		codec:             codec,
		disallowedSpecial: special,
	}, nil
}

// EncodingName returns the configured tiktoken vocabulary name.
func (t *TiktokenTokenizer) EncodingName() string {
	return t.encodingName
}

// Encode converts UTF-8 text into vocabulary token identifiers.
func (t *TiktokenTokenizer) Encode(text string) ([]TokenID, error) {
	if err := t.validateText(text); err != nil {
		return nil, err
	}
	ids, _, err := t.codec.Encode(text)
	if err != nil {
		return nil, fmt.Errorf("encode text with %s: %w", t.encodingName, err)
	}
	tokens := make([]TokenID, len(ids))
	for index, id := range ids {
		tokens[index] = TokenID(id)
	}
	return tokens, nil
}

// Tokenize returns IDs and their exact source pieces in source order.
func (t *TiktokenTokenizer) Tokenize(text string) ([]Token, error) {
	if err := t.validateText(text); err != nil {
		return nil, err
	}
	ids, pieces, err := t.codec.Encode(text)
	if err != nil {
		return nil, fmt.Errorf("tokenize text with %s: %w", t.encodingName, err)
	}
	if len(ids) != len(pieces) {
		return nil, fmt.Errorf("tokenize text with %s: token IDs and pieces differ", t.encodingName)
	}
	tokens := make([]Token, len(ids))
	for index := range ids {
		tokens[index] = Token{ID: TokenID(ids[index]), Text: pieces[index]}
	}
	return tokens, nil
}

// Decode converts vocabulary token identifiers back into text.
func (t *TiktokenTokenizer) Decode(tokens []TokenID) (string, error) {
	ids := make([]uint, len(tokens))
	for index, token := range tokens {
		ids[index] = uint(token)
	}
	text, err := t.codec.Decode(ids)
	if err != nil {
		return "", fmt.Errorf("decode tokens with %s: %w", t.encodingName, err)
	}
	// Incomplete UTF-8 byte sequences are decoded with replacement characters
	// characters. A fixed token window can expose such a sequence at its edge.
	return strings.ToValidUTF8(text, "\uFFFD"), nil
}

// Count returns the number of vocabulary tokens in UTF-8 text.
func (t *TiktokenTokenizer) Count(text string) (int, error) {
	if err := t.validateText(text); err != nil {
		return 0, err
	}
	count, err := t.codec.Count(text)
	if err != nil {
		return 0, fmt.Errorf("count text with %s: %w", t.encodingName, err)
	}
	return count, nil
}

// Split applies one fixed token window using this tokenizer's vocabulary.
// It exposes plain strings so embedding consumers do not depend on corpus token types.
func (t *TiktokenTokenizer) Split(text string, size, overlap int) ([]string, error) {
	chunker, err := newTokenChunker(ChunkPolicy{Size: size, Overlap: overlap}, t)
	if err != nil {
		return nil, err
	}
	chunks, err := chunker.Chunk(context.Background(), text)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(chunks))
	for index, chunk := range chunks {
		result[index] = chunk.Text
	}
	return result, nil
}

func (t *TiktokenTokenizer) validateText(text string) error {
	if !utf8.ValidString(text) {
		return errors.New("tokenizer input must be valid UTF-8")
	}
	for _, special := range t.disallowedSpecial {
		if strings.Contains(text, special) {
			return fmt.Errorf("%w: %q", ErrDisallowedSpecialToken, special)
		}
	}
	return nil
}

func resolveTiktokenEncoding(encodingName string) (tiktoken.Encoding, []string, error) {
	// Special token literals come from OpenAI tiktoken's public encoding
	// constructors. Its encode() default rejects these literals in normal text.
	switch encodingName {
	case "o200k_base":
		return tiktoken.O200kBase, []string{"<|endoftext|>", "<|endofprompt|>"}, nil
	case "cl100k_base":
		return tiktoken.Cl100kBase, []string{
			"<|endoftext|>", "<|fim_prefix|>", "<|fim_middle|>", "<|fim_suffix|>", "<|endofprompt|>",
		}, nil
	case "r50k_base":
		return tiktoken.R50kBase, []string{"<|endoftext|>"}, nil
	case "p50k_base":
		return tiktoken.P50kBase, []string{"<|endoftext|>"}, nil
	case "p50k_edit":
		return tiktoken.P50kEdit, []string{
			"<|endoftext|>", "<|fim_prefix|>", "<|fim_middle|>", "<|fim_suffix|>",
		}, nil
	default:
		return "", nil, fmt.Errorf("unsupported tiktoken encoding %q", encodingName)
	}
}
