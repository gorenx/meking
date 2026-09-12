package agent

import (
	"context"
	"errors"

	provider "github.com/memoria-space/meking/agent"
	"github.com/memoria-space/meking/knowledge/extraction"
)

// model is the minimum Agent capability consumed by Extraction's
// outbound adapter.
type model interface {
	Complete(context.Context, provider.CompletionRequest) (provider.CompletionResponse, error)
}

// Completion maps Agent messages into the Extraction consumer contract.
type Completion struct {
	model model
}

func NewCompletion(model model) (*Completion, error) {
	if model == nil {
		return nil, errors.New("create Knowledge Extraction Agent Completion: client is required")
	}
	return &Completion{model: model}, nil
}

func (c *Completion) Complete(
	ctx context.Context,
	request extraction.CompletionRequest,
) (extraction.CompletionResponse, error) {
	messages := make([]provider.CompletionMessage, len(request.Messages))
	for index, message := range request.Messages {
		messages[index] = provider.CompletionMessage{
			Role:    provider.CompletionRole(message.Role),
			Content: message.Content,
		}
	}
	providerRequest := provider.CompletionRequest{
		Messages:            messages,
		MaxCompletionTokens: request.MaxCompletionTokens,
	}
	if len(request.Schema) != 0 {
		providerRequest.ResponseFormat = provider.CompletionResponseJSONSchema
		providerRequest.JSONSchema = &provider.JSONSchema{
			Name:       request.SchemaName,
			Definition: append([]byte(nil), request.Schema...),
		}
	}
	response, err := c.model.Complete(ctx, providerRequest)
	if err != nil {
		return extraction.CompletionResponse{}, err
	}
	return extraction.CompletionResponse{Content: response.Content}, nil
}

var _ extraction.CompletionModel = (*Completion)(nil)
