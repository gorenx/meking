package adapter

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/semantic"
)

type embeddingStub struct {
	result semantic.EmbeddingBatch
	err    error
}

func (s embeddingStub) Embed(context.Context, []string) (semantic.EmbeddingBatch, error) {
	return s.result, s.err
}

func TestEmbedderServesSharedQueryContracts(t *testing.T) {
	embedder, err := NewEmbedder("embedding-model", embeddingStub{
		result: semantic.EmbeddingBatch{Vectors: [][]float64{{1, 2}}},
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}
	question, err := embedder.EmbedQuestion(t.Context(), "question")
	if err != nil {
		t.Fatalf("EmbedQuestion() error = %v", err)
	}
	hypothetical, err := embedder.Embed(t.Context(), "hypothetical")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if embedder.Model() != "embedding-model" ||
		!reflect.DeepEqual(question, []float64{1, 2}) ||
		!reflect.DeepEqual(hypothetical, []float64{1, 2}) {
		t.Fatalf("shared embedding = model:%q question:%v hypothetical:%v",
			embedder.Model(), question, hypothetical)
	}
	question[0] = 9
	if hypothetical[0] != 1 {
		t.Fatal("Embedder returned aliased vectors")
	}
}

func TestEmbedderRejectsInvalidModelResponse(t *testing.T) {
	embedder, err := NewEmbedder("embedding-model", embeddingStub{
		result: semantic.EmbeddingBatch{Vectors: [][]float64{{math.NaN()}}},
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}
	_, err = embedder.EmbedQuestion(t.Context(), "question")
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != querybase.FailureInvalidModelResponse {
		t.Fatalf("error = %v, want invalid_model_response", err)
	}
}
