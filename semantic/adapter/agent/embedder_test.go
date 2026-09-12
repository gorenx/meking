package agent

import (
	"context"
	"reflect"
	"testing"

	provider "github.com/memoria-space/meking/agent"
)

type embeddingClientStub struct {
	request  provider.EmbeddingRequest
	response provider.EmbeddingResponse
	err      error
}

func (client *embeddingClientStub) Embed(
	_ context.Context,
	request provider.EmbeddingRequest,
) (provider.EmbeddingResponse, error) {
	client.request = request
	return client.response, client.err
}

func TestEmbedderMapsAgentResponse(t *testing.T) {
	client := &embeddingClientStub{
		response: provider.EmbeddingResponse{Vectors: [][]float64{{1, 2}, {3, 4}}},
	}
	embedder, err := NewEmbedder(client)
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}

	response, err := embedder.Embed(t.Context(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if !reflect.DeepEqual(client.request.Texts, []string{"first", "second"}) {
		t.Fatalf("request texts = %#v", client.request.Texts)
	}
	if !reflect.DeepEqual(response.Vectors, client.response.Vectors) {
		t.Fatalf("vectors = %#v", response.Vectors)
	}
}

func TestNewEmbedderRequiresClient(t *testing.T) {
	if _, err := NewEmbedder(nil); err == nil {
		t.Fatal("NewEmbedder(nil) error = nil")
	}
}
