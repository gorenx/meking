package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAIEmbeddingMapsOrderedBatch(t *testing.T) {
	var authorization string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		if request.URL.Path != "/v1/embeddings" {
			t.Errorf("request path = %q, want /v1/embeddings", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"model":"embedding-test",
			"data":[
				{"object":"embedding","index":1,"embedding":[1.5,2.5]},
				{"object":"embedding","index":0,"embedding":[3.5,4.5]}
			],
			"usage":{"prompt_tokens":4,"total_tokens":4}
		}`))
	}))
	defer server.Close()

	adapter := mustOpenAIEmbedding(t, OpenAIEmbeddingConfig{
		APIKey: "secret", Model: "embedding-test",
		BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	response, err := adapter.Embed(t.Context(), EmbeddingRequest{Texts: []string{"first", "second"}})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if authorization != "Bearer secret" {
		t.Fatalf("authorization = %q", authorization)
	}
	if got, want := body["model"], "embedding-test"; got != want {
		t.Fatalf("model = %#v, want %#v", got, want)
	}
	if got, want := body["encoding_format"], "float"; got != want {
		t.Fatalf("encoding_format = %#v, want %#v", got, want)
	}
	if got, want := body["input"], []any{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
	want := [][]float64{{1.5, 2.5}, {3.5, 4.5}}
	if !reflect.DeepEqual(response.Vectors, want) {
		t.Fatalf("vectors = %#v, want %#v", response.Vectors, want)
	}
}

func TestOpenAIEmbeddingSkipsEmptyBatch(t *testing.T) {
	adapter := mustOpenAIEmbedding(t, OpenAIEmbeddingConfig{APIKey: "key", Model: "model"})
	response, err := adapter.Embed(t.Context(), EmbeddingRequest{})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if response.Vectors == nil || len(response.Vectors) != 0 {
		t.Fatalf("vectors = %#v, want non-nil empty slice", response.Vectors)
	}
}

func TestOpenAIEmbeddingKeepsSDKRetriesDisabled(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, `{"error":{"message":"temporary","type":"server_error"}}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := mustOpenAIEmbedding(t, OpenAIEmbeddingConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	_, err := adapter.Embed(t.Context(), EmbeddingRequest{Texts: []string{"text"}})
	if err == nil || !strings.Contains(err.Error(), "OpenAI embedding") {
		t.Fatalf("Embed() error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want SDK retries disabled", requests.Load())
	}
}

func TestOpenAIEmbeddingPropagatesCancellation(t *testing.T) {
	adapter := mustOpenAIEmbedding(t, OpenAIEmbeddingConfig{APIKey: "key", Model: "model"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := adapter.Embed(ctx, EmbeddingRequest{Texts: []string{"text"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Embed() error = %v, want context.Canceled", err)
	}
}

func TestNewOpenAIEmbeddingValidatesConfig(t *testing.T) {
	tests := []struct {
		name   string
		config OpenAIEmbeddingConfig
		match  string
	}{
		{name: "API key", config: OpenAIEmbeddingConfig{Model: "model"}, match: "API key"},
		{name: "unresolved API key", config: OpenAIEmbeddingConfig{APIKey: "${MEKING_API_KEY}", Model: "model"}, match: "not resolved"},
		{name: "model", config: OpenAIEmbeddingConfig{APIKey: "key"}, match: "embedding model"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewOpenAIEmbedding(test.config); err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("NewOpenAIEmbedding() error = %v, want %q", err, test.match)
			}
		})
	}
}

func mustOpenAIEmbedding(t *testing.T, config OpenAIEmbeddingConfig) *OpenAIEmbedding {
	t.Helper()
	adapter, err := NewOpenAIEmbedding(config)
	if err != nil {
		t.Fatalf("NewOpenAIEmbedding() error = %v", err)
	}
	return adapter
}
