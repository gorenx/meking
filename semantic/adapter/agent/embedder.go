// Package agent adapts external Agent embeddings to Semantic's Embedder port.
package agent

import (
	"context"
	"errors"

	provider "github.com/memoria-space/meking/agent"
	"github.com/memoria-space/meking/semantic"
)

type embeddingClient interface {
	Embed(context.Context, provider.EmbeddingRequest) (provider.EmbeddingResponse, error)
}

type Embedder struct {
	client embeddingClient
}

func NewEmbedder(client embeddingClient) (*Embedder, error) {
	if client == nil {
		return nil, errors.New("create Semantic Agent Embedder: client is required")
	}
	return &Embedder{client: client}, nil
}

func (embedder *Embedder) Embed(
	ctx context.Context,
	texts []string,
) (semantic.EmbeddingBatch, error) {
	response, err := embedder.client.Embed(ctx, provider.EmbeddingRequest{
		Texts: append([]string(nil), texts...),
	})
	if err != nil {
		return semantic.EmbeddingBatch{}, err
	}
	vectors := make([][]float64, len(response.Vectors))
	for index, vector := range response.Vectors {
		vectors[index] = append([]float64(nil), vector...)
	}
	return semantic.EmbeddingBatch{Vectors: vectors}, nil
}

var _ semantic.Embedder = (*Embedder)(nil)
