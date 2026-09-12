package textunits

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTiktokenTokenizerEncodesAndCounts(t *testing.T) {
	// Source: github.com/tiktoken-go/tokenizer v0.8.1 TestO200kBase.
	tokenizer, err := NewTiktokenTokenizer(DefaultEncodingModel)
	if err != nil {
		t.Fatalf("NewTiktokenTokenizer() error = %v", err)
	}
	if tokenizer.EncodingName() != DefaultEncodingModel {
		t.Errorf("EncodingName() = %q, want %q", tokenizer.EncodingName(), DefaultEncodingModel)
	}
	tokens, err := tokenizer.Encode("hello world")
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if expected := []TokenID{24912, 2375}; !reflect.DeepEqual(tokens, expected) {
		t.Errorf("Encode() = %v, want %v", tokens, expected)
	}
	count, err := tokenizer.Count("hello world")
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != len(tokens) {
		t.Errorf("Count() = %d, want %d", count, len(tokens))
	}
}

func TestTiktokenTokenizerRejectsUnsupportedEncoding(t *testing.T) {
	_, err := NewTiktokenTokenizer("unknown")
	if err == nil || !strings.Contains(err.Error(), "unsupported tiktoken encoding") {
		t.Fatalf("NewTiktokenTokenizer() error = %v, want unsupported encoding", err)
	}
}

func TestTiktokenTokenizerRejectsDisallowedSpecialToken(t *testing.T) {
	tokenizer, err := NewTiktokenTokenizer(DefaultEncodingModel)
	if err != nil {
		t.Fatalf("NewTiktokenTokenizer() error = %v", err)
	}
	_, err = tokenizer.Encode("before <|endoftext|> after")
	if !errors.Is(err, ErrDisallowedSpecialToken) {
		t.Fatalf("Encode() error = %v, want ErrDisallowedSpecialToken", err)
	}
	_, err = tokenizer.Count("<|endofprompt|>")
	if !errors.Is(err, ErrDisallowedSpecialToken) {
		t.Fatalf("Count() error = %v, want ErrDisallowedSpecialToken", err)
	}
}

func TestTiktokenTokenizerRejectsInvalidUTF8(t *testing.T) {
	tokenizer, err := NewTiktokenTokenizer(DefaultEncodingModel)
	if err != nil {
		t.Fatalf("NewTiktokenTokenizer() error = %v", err)
	}
	_, err = tokenizer.Encode(string([]byte{0xff}))
	if err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("Encode() error = %v, want UTF-8 error", err)
	}
}

func TestTiktokenTokenizerDecodeRejectsUnknownToken(t *testing.T) {
	tokenizer, err := NewTiktokenTokenizer(DefaultEncodingModel)
	if err != nil {
		t.Fatalf("NewTiktokenTokenizer() error = %v", err)
	}
	_, err = tokenizer.Decode([]TokenID{999999999})
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("Decode() error = %v, want invalid token", err)
	}
}

func TestTiktokenTokenizerReplacesIncompleteUTF8Token(t *testing.T) {
	tokenizer, err := NewTiktokenTokenizer(DefaultEncodingModel)
	if err != nil {
		t.Fatalf("NewTiktokenTokenizer() error = %v", err)
	}
	tokens, err := tokenizer.Encode("💩")
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if len(tokens) < 2 {
		t.Fatalf("Encode() returned %d token, want a multi-token code point", len(tokens))
	}
	decoded, err := tokenizer.Decode(tokens[:1])
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded != "�" {
		t.Errorf("Decode(first token) = %q, want replacement character", decoded)
	}
}

func TestTiktokenTokenizerSplitExposesPlainTextWindows(t *testing.T) {
	tokenizer, err := NewTiktokenTokenizer("o200k_base")
	if err != nil {
		t.Fatalf("NewTiktokenTokenizer() error = %v", err)
	}
	got, err := tokenizer.Split("one two three", 2, 1)
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	want := []string{"one two", " two three"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Split() = %#v, want %#v", got, want)
	}
}
