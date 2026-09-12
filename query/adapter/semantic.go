package adapter

import (
	"context"
	"errors"
	"math"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/query/internal/modeladapter"
	"github.com/memoria-space/meking/semantic"
)

// Embedder maps one question or hypothetical answer onto the shared Semantic
// embedding API. Basic, Local, and DRIFT own separate retrieval policies but
// require the same model identity and one-vector response contract.
type Embedder struct {
	model    string
	embedder semantic.Embedder
}

// NewEmbedder binds Query embedding calls to the model used by the persisted
// Semantic databases selected by composition.
func NewEmbedder(model string, embedder semantic.Embedder) (*Embedder, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("create Query Embedder: model is required")
	}
	if embedder == nil {
		return nil, errors.New("create Query Embedder: Semantic Embedder is required")
	}
	return &Embedder{model: model, embedder: embedder}, nil
}

func (e *Embedder) Model() string {
	if e == nil {
		return ""
	}
	return e.model
}

func (e *Embedder) EmbedQuestion(ctx context.Context, question string) ([]float64, error) {
	return e.embed(ctx, question)
}

func (e *Embedder) Embed(ctx context.Context, text string) ([]float64, error) {
	return e.embed(ctx, text)
}

func (e *Embedder) embed(ctx context.Context, text string) ([]float64, error) {
	if e == nil || e.embedder == nil {
		return nil, querybase.NewInternalFailure(errors.New("Query Embedder is not configured"))
	}
	batch, err := e.embedder.Embed(ctx, []string{text})
	if err != nil {
		return nil, modeladapter.Failure(err)
	}
	if len(batch.Vectors) != 1 || len(batch.Vectors[0]) == 0 {
		return nil, querybase.NewInvalidModelResponseFailure(
			errors.New("Query embedding response must contain one non-empty vector"),
		)
	}
	vector := append([]float64(nil), batch.Vectors[0]...)
	for _, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, querybase.NewInvalidModelResponseFailure(
				errors.New("Query embedding response contains a non-finite value"),
			)
		}
	}
	return vector, nil
}
