package textunits

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// SentenceBoundaryRequest is the consumer-owned semantic input to an external
// sentence analyzer. Text is supplied directly; the analyzer never receives a
// Project path or a Corpus Document.
type SentenceBoundaryRequest struct {
	Text     string
	Language string
}

// SentenceBoundary is one inclusive Unicode code-point range.
type SentenceBoundary struct {
	StartChar int
	EndChar   int
}

// SentenceAnalyzer finds ordered sentence boundaries without owning
// TextUnitBody creation, token counting, or source identity.
type SentenceAnalyzer interface {
	AnalyzeSentences(ctx context.Context, request SentenceBoundaryRequest) ([]SentenceBoundary, error)
}

// SentenceChunker maps analyzer ranges into corpus-owned chunks. Size and
// overlap are intentionally absent because this strategy emits one sentence
// per chunk.
type SentenceChunker struct {
	analyzer SentenceAnalyzer
	language string
}

// NewSentenceChunker creates a sentence strategy with an explicit analyzer
// and NLTK language identity.
func NewSentenceChunker(analyzer SentenceAnalyzer, language string) (*SentenceChunker, error) {
	if analyzer == nil {
		return nil, errors.New("sentence boundary analyzer is required")
	}
	if !validSentenceLanguage(language) {
		return nil, errors.New("sentence language is invalid")
	}
	return &SentenceChunker{analyzer: analyzer, language: language}, nil
}

// Chunk validates all external ranges before slicing the original UTF-8 text.
func (c *SentenceChunker) Chunk(ctx context.Context, text string) ([]TextChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.analyzer == nil {
		return nil, errors.New("sentence boundary analyzer is required")
	}
	if !utf8.ValidString(text) {
		return nil, errors.New("sentence input must be valid UTF-8")
	}
	boundaries, err := c.analyzer.AnalyzeSentences(ctx, SentenceBoundaryRequest{
		Text: text, Language: c.language,
	})
	if err != nil {
		return nil, fmt.Errorf("analyze sentence boundaries: %w", err)
	}
	runes := []rune(text)
	chunks := make([]TextChunk, 0, len(boundaries))
	previousEnd := -1
	for index, boundary := range boundaries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if boundary.StartChar < 0 || boundary.EndChar < boundary.StartChar || boundary.EndChar >= len(runes) {
			return nil, fmt.Errorf("sentence boundary %d is out of range", index)
		}
		if boundary.StartChar <= previousEnd {
			return nil, fmt.Errorf("sentence boundary %d overlaps its predecessor", index)
		}
		chunkText := string(runes[boundary.StartChar : boundary.EndChar+1])
		if strings.TrimSpace(chunkText) == "" {
			return nil, fmt.Errorf("sentence boundary %d contains only whitespace", index)
		}
		chunks = append(chunks, TextChunk{
			Text: chunkText, StartChar: boundary.StartChar, EndChar: boundary.EndChar + 1,
		})
		previousEnd = boundary.EndChar
	}
	return chunks, nil
}

func validSentenceLanguage(language string) bool {
	if len(language) < 2 || len(language) > 32 {
		return false
	}
	for index, value := range language {
		if value >= 'a' && value <= 'z' || index > 0 && value == '_' {
			continue
		}
		return false
	}
	return true
}
