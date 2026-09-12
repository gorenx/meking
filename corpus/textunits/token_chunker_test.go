package textunits

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestChunkPolicyValidate(t *testing.T) {
	valid := []ChunkPolicy{{Size: 10}, {Size: 10, Overlap: 2}}
	for _, policy := range valid {
		if err := policy.Validate(); err != nil {
			t.Fatalf("Validate(%#v) error = %v", policy, err)
		}
	}
	invalid := []ChunkPolicy{{}, {Size: 10, Overlap: -1}, {Size: 10, Overlap: 10}, {Size: 10, Overlap: 11}}
	for _, policy := range invalid {
		if err := policy.Validate(); err == nil {
			t.Fatalf("Validate(%#v) expected error", policy)
		}
	}
}

func TestTokenChunkerReturnsExactHalfOpenRanges(t *testing.T) {
	chunker, err := newTokenChunker(ChunkPolicy{Size: 4, Overlap: 2}, runeTokenizer{})
	if err != nil {
		t.Fatalf("newTokenChunker() error = %v", err)
	}
	chunks, err := chunker.Chunk(t.Context(), "abcdefgh")
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	want := []TextChunk{
		{Text: "abcd", StartChar: 0, EndChar: 4},
		{Text: "cdef", StartChar: 2, EndChar: 6},
		{Text: "efgh", StartChar: 4, EndChar: 8},
	}
	if !reflect.DeepEqual(chunks, want) {
		t.Fatalf("Chunk() = %#v, want %#v", chunks, want)
	}
}

func TestTokenChunkerUsesUnicodeCharacterPositions(t *testing.T) {
	chunker, _ := newTokenChunker(ChunkPolicy{Size: 2}, runeTokenizer{})
	chunks, err := chunker.Chunk(t.Context(), "甲乙abc")
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	want := []TextChunk{
		{Text: "甲乙", StartChar: 0, EndChar: 2},
		{Text: "ab", StartChar: 2, EndChar: 4},
		{Text: "c", StartChar: 4, EndChar: 5},
	}
	if !reflect.DeepEqual(chunks, want) {
		t.Fatalf("Chunk() = %#v, want %#v", chunks, want)
	}
}

func TestTokenChunkerRejectsMissingTokenizerAndCancellation(t *testing.T) {
	if _, err := newTokenChunker(ChunkPolicy{Size: 2}, nil); err == nil {
		t.Fatal("newTokenChunker(nil) expected error")
	}
	chunker, _ := newTokenChunker(ChunkPolicy{Size: 2}, runeTokenizer{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := chunker.Chunk(ctx, "text"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Chunk(cancelled) error = %v", err)
	}
}

type runeTokenizer struct{}

func (runeTokenizer) Tokenize(value string) ([]Token, error) {
	tokens := make([]Token, 0, len([]rune(value)))
	for index, character := range []rune(value) {
		tokens = append(tokens, Token{ID: TokenID(index), Text: string(character)})
	}
	return tokens, nil
}
