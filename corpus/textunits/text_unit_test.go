package textunits

import (
	"context"
	"errors"
	"testing"

	"github.com/memoria-space/meking/corpus/text"
)

func TestTextUnitBuilderReusesOneChunkingAcrossTexts(t *testing.T) {
	builder, err := NewTextUnitBuilder(Chunking{
		Type: TokenChunking, Size: 1, EncodingModel: "o200k_base",
	}, nil)
	if err != nil {
		t.Fatalf("NewTextUnitBuilder() error = %v", err)
	}
	values := []text.Text{
		{ID: "10000000-0000-4000-8000-000000000001", Body: "first second"},
		{ID: "10000000-0000-4000-8000-000000000002", Body: "third fourth"},
	}
	results := make([][]TextUnit, len(values))
	for index, value := range values {
		results[index], err = builder.Build(t.Context(), value)
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
	}
	if len(results) != 2 || len(results[0]) != 2 || len(results[1]) != 2 ||
		results[0][0].TextUnit.Text != "first" || results[0][1].TextUnit.Text != " second" ||
		results[1][1].EndIndex != len([]rune(values[1].Body)) {
		t.Fatalf("Build results = %#v", results)
	}
}

func TestTextUnitIdentityDependsOnlyOnFinalText(t *testing.T) {
	first, err := NewTextUnitBody("same")
	if err != nil {
		t.Fatalf("NewTextUnitBody() error = %v", err)
	}
	second, err := NewTextUnitBody("same")
	if err != nil {
		t.Fatalf("NewTextUnitBody() error = %v", err)
	}
	if first != second {
		t.Fatalf("equal content TextUnits = %#v and %#v", first, second)
	}
}

func TestTextUnitBuilderRequiresSentenceAnalyzerOnlyForSentenceChunking(t *testing.T) {
	_, err := NewTextUnitBuilder(Chunking{
		Type: SentenceChunking, EncodingModel: "o200k_base", SentenceLanguage: "english",
	}, nil)
	if err == nil {
		t.Fatal("NewTextUnitBuilder() expected sentence analyzer error")
	}
}

func TestTextUnitBuilderPropagatesCancellation(t *testing.T) {
	builder, err := NewTextUnitBuilder(Chunking{
		Type: TokenChunking, Size: 1, EncodingModel: "o200k_base",
	}, nil)
	if err != nil {
		t.Fatalf("NewTextUnitBuilder() error = %v", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	value := text.Text{ID: "10000000-0000-4000-8000-000000000003", Body: "Sentence."}
	if _, err := builder.Build(cancelled, value); !errors.Is(err, context.Canceled) {
		t.Fatalf("Build(cancelled) error = %v", err)
	}
}
