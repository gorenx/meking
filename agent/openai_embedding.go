package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenAIEmbeddingConfig contains the provider settings needed for embeddings.
type OpenAIEmbeddingConfig struct {
	// APIKey authenticates requests and must never enter logs or generated output.
	APIKey string
	// Model is the configured OpenAI embedding model sent with every request.
	Model string
	// BaseURL optionally selects a trusted compatible endpoint or test server.
	BaseURL string
	// HTTPClient optionally supplies transport, timeout, and test behavior.
	HTTPClient *http.Client
}

// OpenAIEmbedding calls one OpenAI-compatible Agent embedding endpoint.
type OpenAIEmbedding struct {
	// client owns the official SDK transport and authentication options.
	client openai.Client
	// model is the validated identifier mapped into each Embeddings request.
	model string
}

// NewOpenAIEmbedding creates an Agent client with SDK retries disabled.
func NewOpenAIEmbedding(config OpenAIEmbeddingConfig) (*OpenAIEmbedding, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Model = strings.TrimSpace(config.Model)
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	if config.APIKey == "" {
		return nil, errors.New("OpenAI API key is required")
	}
	if config.APIKey == "<API_KEY>" ||
		(strings.HasPrefix(config.APIKey, "${") && strings.HasSuffix(config.APIKey, "}")) {
		return nil, errors.New("OpenAI API key environment reference is not resolved")
	}
	if config.Model == "" {
		return nil, errors.New("OpenAI embedding model is required")
	}

	options := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
		option.WithMaxRetries(0),
	}
	if config.BaseURL != "" {
		options = append(options, option.WithBaseURL(config.BaseURL))
	}
	if config.HTTPClient != nil {
		options = append(options, option.WithHTTPClient(config.HTTPClient))
	}

	return &OpenAIEmbedding{
		client: openai.NewClient(options...),
		model:  config.Model,
	}, nil
}

// Embed sends one ordered batch and preserves response data order.
func (e *OpenAIEmbedding) Embed(ctx context.Context, request EmbeddingRequest) (EmbeddingResponse, error) {
	if len(request.Texts) == 0 {
		return EmbeddingResponse{Vectors: [][]float64{}}, nil
	}
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: append([]string(nil), request.Texts...),
		},
		Model:          openai.EmbeddingModel(e.model),
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	}
	response, err := e.client.Embeddings.New(ctx, params)
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("OpenAI embedding: %w", newProviderError(err))
	}

	vectors := make([][]float64, len(response.Data))
	for index, item := range response.Data {
		vectors[index] = append([]float64(nil), item.Embedding...)
	}
	return EmbeddingResponse{Vectors: vectors}, nil
}
