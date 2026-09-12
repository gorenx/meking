package textunits

import (
	"errors"
	"fmt"
	"strings"
)

// ChunkingType identifies the algorithm used to construct ordered TextUnits.
type ChunkingType string

const (
	// TokenChunking uses a fixed token window and overlap.
	TokenChunking ChunkingType = "tokens"
	// SentenceChunking emits one analyzer-selected sentence per TextUnitBody.
	SentenceChunking ChunkingType = "sentence"
	// DefaultSentenceLanguage is the first-release sentence boundary policy.
	DefaultSentenceLanguage = "english"
)

// ErrInvalidChunking identifies settings that cannot deterministically build
// TextUnits.
var ErrInvalidChunking = errors.New("invalid TextUnit Chunking")

// Chunking records the complete settings needed to reproduce TextUnits.
type Chunking struct {
	Type             ChunkingType
	Size             int
	Overlap          int
	EncodingModel    string
	SentenceLanguage string
}

// ValidateChunking checks the complete configuration needed to construct and
// later tokenize TextUnits.
func ValidateChunking(chunking Chunking) error {
	if strings.TrimSpace(chunking.EncodingModel) == "" ||
		chunking.EncodingModel != strings.TrimSpace(chunking.EncodingModel) {
		return fmt.Errorf("%w: EncodingModel is required", ErrInvalidChunking)
	}
	switch chunking.Type {
	case TokenChunking:
		if chunking.Size <= 0 {
			return fmt.Errorf("%w: token Size must be positive", ErrInvalidChunking)
		}
		if chunking.Overlap < 0 {
			return fmt.Errorf("%w: token Overlap must be non-negative", ErrInvalidChunking)
		}
		if chunking.Overlap >= chunking.Size {
			return fmt.Errorf("%w: token Overlap must be smaller than Size", ErrInvalidChunking)
		}
		if chunking.SentenceLanguage != "" {
			return fmt.Errorf("%w: token strategy cannot have SentenceLanguage", ErrInvalidChunking)
		}
	case SentenceChunking:
		if chunking.Size != 0 || chunking.Overlap != 0 {
			return fmt.Errorf("%w: sentence Size and Overlap must be zero", ErrInvalidChunking)
		}
		if !validSentenceLanguage(chunking.SentenceLanguage) {
			return fmt.Errorf("%w: sentence language is invalid", ErrInvalidChunking)
		}
	default:
		return fmt.Errorf("%w: unsupported Type %q", ErrInvalidChunking, chunking.Type)
	}
	return nil
}
