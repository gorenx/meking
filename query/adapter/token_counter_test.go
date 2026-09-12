package adapter

import (
	"errors"
	"testing"
)

type tokenCounterStub struct {
	count int
	err   error
	texts []string
}

func (s *tokenCounterStub) Count(text string) (int, error) {
	s.texts = append(s.texts, text)
	return s.count, s.err
}

func TestTokenCounterUsesConfiguredTokenizer(t *testing.T) {
	tokens := &tokenCounterStub{count: 3}
	counter, err := NewTokenCounter(tokens)
	if err != nil {
		t.Fatalf("NewTokenCounter() error = %v", err)
	}
	count, err := counter.Count(t.Context(), "1", "Alpha and Beta")
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 3 || len(tokens.texts) != 1 || tokens.texts[0] != "Alpha and Beta" {
		t.Fatalf("Count() = %d, texts = %#v", count, tokens.texts)
	}
}

func TestTokenCounterRejectsInvalidInputAndPropagatesFailure(t *testing.T) {
	want := errors.New("count failed")
	counter, err := NewTokenCounter(&tokenCounterStub{err: want})
	if err != nil {
		t.Fatalf("NewTokenCounter() error = %v", err)
	}
	if _, err := counter.Count(t.Context(), "1", "text"); !errors.Is(err, want) {
		t.Fatalf("Count(tokenizer failure) error = %v", err)
	}
	if _, err := counter.Count(t.Context(), "invalid", "text"); err == nil {
		t.Fatal("Count(invalid CorporaID) error = nil")
	}
	if _, err := NewTokenCounter(nil); err == nil {
		t.Fatal("NewTokenCounter(nil) error = nil")
	}
}
