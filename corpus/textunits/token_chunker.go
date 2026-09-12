package textunits

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"
)

type ChunkPolicy struct {
	Size    int
	Overlap int
}

func (p ChunkPolicy) Validate() error {
	if p.Size <= 0 {
		return errors.New("chunk size must be positive")
	}
	if p.Overlap < 0 || p.Overlap >= p.Size {
		return errors.New("chunk overlap must be non-negative and smaller than size")
	}
	return nil
}

// TextChunk is a transient half-open range selected from normalized Text.
type TextChunk struct {
	Text      string
	StartChar int
	EndChar   int
}

type Chunker interface {
	Chunk(ctx context.Context, text string) ([]TextChunk, error)
}

type tokenizer interface {
	Tokenize(string) ([]Token, error)
}

type tokenChunker struct {
	policy    ChunkPolicy
	tokenizer tokenizer
}

func newTokenChunker(policy ChunkPolicy, tokenizer tokenizer) (*tokenChunker, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if tokenizer == nil {
		return nil, errors.New("tokenizer is required")
	}
	return &tokenChunker{policy: policy, tokenizer: tokenizer}, nil
}

func (c *tokenChunker) Chunk(ctx context.Context, value string) ([]TextChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.policy.Validate(); err != nil {
		return nil, fmt.Errorf("validate chunk policy: %w", err)
	}
	if c.tokenizer == nil {
		return nil, errors.New("tokenizer is required")
	}
	tokens, err := c.tokenizer.Tokenize(value)
	if err != nil {
		return nil, fmt.Errorf("tokenize Text for chunking: %w", err)
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	byteOffsets := make([]int, len(tokens)+1)
	for index, token := range tokens {
		byteOffsets[index+1] = byteOffsets[index] + len(token.Text)
	}
	if byteOffsets[len(tokens)] != len(value) {
		return nil, errors.New("tokenizer pieces do not reconstruct Text")
	}
	runeOffsets := make(map[int]int)
	runeIndex := 0
	for byteIndex := range value {
		runeOffsets[byteIndex] = runeIndex
		runeIndex++
	}
	runeOffsets[len(value)] = utf8.RuneCountInString(value)

	chunks := make([]TextChunk, 0)
	for start := 0; start < len(tokens); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+c.policy.Size, len(tokens))
		for end > start {
			if _, valid := runeOffsets[byteOffsets[end]]; valid {
				break
			}
			end--
		}
		if end == start {
			return nil, fmt.Errorf("token window at token %d cannot end on a Unicode character boundary", start)
		}
		startChar, valid := runeOffsets[byteOffsets[start]]
		if !valid {
			return nil, fmt.Errorf("token window at token %d does not start on a Unicode character boundary", start)
		}
		endChar := runeOffsets[byteOffsets[end]]
		chunkText := value[byteOffsets[start]:byteOffsets[end]]
		chunks = append(chunks, TextChunk{Text: chunkText, StartChar: startChar, EndChar: endChar})
		if end == len(tokens) {
			break
		}
		next := end - c.policy.Overlap
		if next <= start {
			next = start + 1
		}
		for next < end {
			if _, valid := runeOffsets[byteOffsets[next]]; valid {
				break
			}
			next++
		}
		start = next
	}
	return chunks, nil
}
