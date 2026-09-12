package textunits

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type sentenceAnalyzerStub struct {
	boundaries []SentenceBoundary
	err        error
	request    *SentenceBoundaryRequest
}

func (s *sentenceAnalyzerStub) AnalyzeSentences(
	_ context.Context,
	request SentenceBoundaryRequest,
) ([]SentenceBoundary, error) {
	s.request = &request
	return append([]SentenceBoundary(nil), s.boundaries...), s.err
}

func TestSentenceChunkerMapsCodePointRangesToOriginalText(t *testing.T) {
	t.Parallel()
	analyzer := &sentenceAnalyzerStub{boundaries: []SentenceBoundary{
		{StartChar: 3, EndChar: 13},
		{StartChar: 15, EndChar: 33},
	}}
	chunker, err := NewSentenceChunker(analyzer, "english")
	if err != nil {
		t.Fatalf("NewSentenceChunker() error = %v", err)
	}
	text := "   Café works. 再见！ Final sentence."
	chunks, err := chunker.Chunk(t.Context(), text)
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	want := []TextChunk{
		{Text: "Café works.", StartChar: 3, EndChar: 14},
		{Text: "再见！ Final sentence.", StartChar: 15, EndChar: 34},
	}
	if !reflect.DeepEqual(chunks, want) {
		t.Fatalf("Chunk() = %#v, want %#v", chunks, want)
	}
	if analyzer.request == nil || analyzer.request.Text != text || analyzer.request.Language != "english" {
		t.Fatalf("AnalyzeSentences() request = %#v", analyzer.request)
	}
}

func TestSentenceChunkerRejectsInvalidDependenciesAndBoundaries(t *testing.T) {
	t.Parallel()
	if _, err := NewSentenceChunker(nil, "english"); err == nil {
		t.Fatal("NewSentenceChunker(nil) expected error")
	}
	if _, err := NewSentenceChunker(&sentenceAnalyzerStub{}, "English"); err == nil {
		t.Fatal("NewSentenceChunker(English) expected error")
	}
	tests := []struct {
		name       string
		boundaries []SentenceBoundary
	}{
		{name: "range", boundaries: []SentenceBoundary{{StartChar: 0, EndChar: 99}}},
		{name: "overlap", boundaries: []SentenceBoundary{{StartChar: 0, EndChar: 2}, {StartChar: 2, EndChar: 4}}},
		{name: "whitespace", boundaries: []SentenceBoundary{{StartChar: 3, EndChar: 3}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunker, err := NewSentenceChunker(&sentenceAnalyzerStub{boundaries: test.boundaries}, "english")
			if err != nil {
				t.Fatalf("NewSentenceChunker() error = %v", err)
			}
			if _, err := chunker.Chunk(t.Context(), "One two."); err == nil {
				t.Fatal("Chunk() expected validation error")
			}
		})
	}
}

func TestSentenceChunkerPropagatesFailureAndCancellation(t *testing.T) {
	t.Parallel()
	expected := errors.New("analysis failed")
	chunker, err := NewSentenceChunker(&sentenceAnalyzerStub{err: expected}, "english")
	if err != nil {
		t.Fatalf("NewSentenceChunker() error = %v", err)
	}
	_, err = chunker.Chunk(t.Context(), "Text.")
	if !errors.Is(err, expected) || !strings.Contains(err.Error(), "analyze sentence boundaries") {
		t.Fatalf("Chunk() error = %v", err)
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = chunker.Chunk(cancelled, "Text.")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Chunk(cancelled) error = %v", err)
	}
}
