package textunits

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus/text"
)

// TextUnitID is the SHA-512 identity of final TextUnitBody text.
type TextUnitID string

// TextUnitBody is immutable, globally reusable text content.
type TextUnitBody struct {
	ID   TextUnitID
	Text string
}

// TextUnit records one TextUnitBody derived from an exact character range in Text.
type TextUnit struct {
	TextUnit   TextUnitBody
	StartIndex int // for text, start index is start char, and end index is end char
	EndIndex   int
	TokenCount int
}

func (u TextUnit) ID() TextUnitID {
	return u.TextUnit.ID
}

type tokenCounter interface {
	Count(string) (int, error)
}

// TextUnitBuilder applies one command's Chunking runtime to each Text processed
// by that command.
type TextUnitBuilder struct {
	chunker   Chunker
	tokenizer tokenCounter
}

func NewTextUnitBuilder(
	chunking Chunking,
	sentenceAnalyzer SentenceAnalyzer,
) (*TextUnitBuilder, error) {
	if err := ValidateChunking(chunking); err != nil {
		return nil, err
	}
	newTokenizer, err := NewTiktokenTokenizer(chunking.EncodingModel)
	if err != nil {
		return nil, err
	}
	var chunker Chunker
	switch chunking.Type {
	case TokenChunking:
		chunker, err = newTokenChunker(
			ChunkPolicy{Size: chunking.Size, Overlap: chunking.Overlap},
			newTokenizer,
		)
	case SentenceChunking:
		if sentenceAnalyzer == nil {
			return nil, errors.New("sentence boundary analyzer is required")
		}
		chunker, err = NewSentenceChunker(sentenceAnalyzer, chunking.SentenceLanguage)
	default:
		return nil, fmt.Errorf("%w: unsupported Type %q", ErrInvalidChunking, chunking.Type)
	}
	if err != nil {
		return nil, err
	}
	return &TextUnitBuilder{chunker: chunker, tokenizer: newTokenizer}, nil
}

func (b *TextUnitBuilder) Build(ctx context.Context, value text.Text) ([]TextUnit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil || b.chunker == nil || b.tokenizer == nil {
		return nil, errors.New("TextUnitBuilder is not initialized")
	}
	return buildTextUnits(ctx, value, b.chunker, b.tokenizer)
}

func buildTextUnits(
	ctx context.Context,
	value text.Text,
	chunker Chunker,
	tokenizer tokenCounter,
) ([]TextUnit, error) {
	if err := text.ValidateID(value.ID); err != nil {
		return nil, fmt.Errorf("build TextUnits: %w", err)
	}
	chunks, err := chunker.Chunk(ctx, value.Body)
	if err != nil {
		return nil, fmt.Errorf("chunk Text %s: %w", value.ID, err)
	}
	outputs := make([]TextUnit, len(chunks))
	for index, chunk := range chunks {
		unit, err := NewTextUnitBody(chunk.Text)
		if err != nil {
			return nil, fmt.Errorf("build TextUnit %d for Text %s: %w", index, value.ID, err)
		}
		tokenCount, err := tokenizer.Count(unit.Text)
		if err != nil {
			return nil, fmt.Errorf("count TextUnit %d for Text %s: %w", index, value.ID, err)
		}
		output := TextUnit{
			TextUnit:   unit,
			StartIndex: chunk.StartChar,
			EndIndex:   chunk.EndChar,
			TokenCount: tokenCount,
		}
		if err = ValidateTextUnitContent(value, output); err != nil {
			return nil, fmt.Errorf("build TextUnit %d for Text %s: %w", index, value.ID, err)
		}
		outputs[index] = output
	}
	return outputs, nil
}

func NewTextUnitBody(value string) (TextUnitBody, error) {
	digest := sha512.Sum512([]byte(value))
	unit := TextUnitBody{ID: TextUnitID(hex.EncodeToString(digest[:])), Text: value}
	if err := ValidateTextUnitBody(unit); err != nil {
		return TextUnitBody{}, err
	}
	return unit, nil
}
