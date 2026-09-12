package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAICompletionConfig configures one OpenAI-compatible Agent endpoint.
type OpenAICompletionConfig struct {
	// APIKey authenticates requests and must never enter logs or generated output.
	APIKey string
	// Model is the configured OpenAI model identifier sent with every request.
	Model string
	// BaseURL optionally selects a trusted compatible endpoint or test server.
	BaseURL string
	// StructuredOutput selects the strongest structured response protocol the
	// configured endpoint supports for consumer-owned JSON contracts.
	StructuredOutput StructuredOutputMode
	// HTTPClient optionally supplies transport, timeout, and test behavior.
	HTTPClient *http.Client
}

// OpenAICompletion calls one OpenAI-compatible Agent completion endpoint.
type OpenAICompletion struct {
	// client owns the official SDK transport and authentication options.
	client openai.Client
	// model is the validated identifier mapped into each Chat Completions request.
	model string
	// structuredOutput controls only structured consumer contracts; ordinary
	// text and streaming requests are unchanged.
	structuredOutput StructuredOutputMode
}

// NewOpenAICompletion creates an Agent client with SDK retries disabled.
func NewOpenAICompletion(config OpenAICompletionConfig) (*OpenAICompletion, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Model = strings.TrimSpace(config.Model)
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	if config.StructuredOutput == "" {
		config.StructuredOutput = StructuredOutputJSONSchema
	}
	if config.APIKey == "" {
		return nil, errors.New("OpenAI API key is required")
	}
	if config.APIKey == "<API_KEY>" ||
		(strings.HasPrefix(config.APIKey, "${") && strings.HasSuffix(config.APIKey, "}")) {
		return nil, errors.New("OpenAI API key environment reference is not resolved")
	}
	if config.Model == "" {
		return nil, errors.New("OpenAI completion model is required")
	}
	if err := config.StructuredOutput.Validate(); err != nil {
		return nil, err
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

	return &OpenAICompletion{
		client:           openai.NewClient(options...),
		model:            config.Model,
		structuredOutput: config.StructuredOutput,
	}, nil
}

// Complete maps text messages to one non-streaming Chat Completions request.
func (c *OpenAICompletion) Complete(ctx context.Context, request CompletionRequest) (CompletionResponse, error) {
	params, err := c.completionParams(request)
	if err != nil {
		return CompletionResponse{}, err
	}
	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("OpenAI chat completion: %w", newProviderError(err))
	}
	if len(response.Choices) == 0 {
		return CompletionResponse{}, errors.New("OpenAI chat completion returned no choices")
	}
	return CompletionResponse{Content: response.Choices[0].Message.Content}, nil
}

// Stream maps text messages to one Chat Completions stream.
func (c *OpenAICompletion) Stream(
	ctx context.Context,
	request CompletionRequest,
	emit CompletionDeltaHandler,
) (CompletionStreamResponse, error) {
	if emit == nil {
		return CompletionStreamResponse{}, errors.New("OpenAI chat completion stream handler is required")
	}
	params, err := c.completionParams(request)
	if err != nil {
		return CompletionStreamResponse{}, err
	}
	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()
	var content strings.Builder
	finished := false
	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			if choice.Index != 0 {
				continue
			}
			if choice.FinishReason != "" {
				finished = true
			}
			delta := choice.Delta.Content
			if delta == "" {
				continue
			}
			content.WriteString(delta)
			if err := emit(delta); err != nil {
				return CompletionStreamResponse{Content: content.String()}, err
			}
		}
	}
	response := CompletionStreamResponse{Content: content.String()}
	if err := stream.Err(); err != nil {
		return response, fmt.Errorf("OpenAI chat completion stream: %w", newProviderError(err))
	}
	if !finished {
		return response, errors.New("OpenAI chat completion stream ended before a finish reason")
	}
	return response, nil
}

func (c *OpenAICompletion) completionParams(request CompletionRequest) (openai.ChatCompletionNewParams, error) {
	if request.MaxCompletionTokens < 0 {
		return openai.ChatCompletionNewParams{}, errors.New("completion max tokens must be non-negative")
	}
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(request.Messages))
	for index, message := range request.Messages {
		switch message.Role {
		case CompletionRoleSystem:
			messages = append(messages, openai.SystemMessage(message.Content))
		case CompletionRoleUser:
			messages = append(messages, openai.UserMessage(message.Content))
		case CompletionRoleAssistant:
			messages = append(messages, openai.AssistantMessage(message.Content))
		default:
			return openai.ChatCompletionNewParams{}, fmt.Errorf("completion message %d: unsupported role %q", index, message.Role)
		}
	}

	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(c.model),
	}
	if request.MaxCompletionTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(request.MaxCompletionTokens))
	}
	switch request.ResponseFormat {
	case CompletionResponseText:
	case CompletionResponseJSONObject:
		if request.JSONSchema != nil {
			return openai.ChatCompletionNewParams{}, errors.New(
				"completion JSON schema is only valid with json_schema response format",
			)
		}
		format := shared.NewResponseFormatJSONObjectParam()
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &format,
		}
	case CompletionResponseJSONSchema:
		if request.JSONSchema == nil {
			return openai.ChatCompletionNewParams{}, errors.New("completion JSON schema is required")
		}
		name := strings.TrimSpace(request.JSONSchema.Name)
		if name == "" {
			return openai.ChatCompletionNewParams{}, errors.New("completion JSON schema name is required")
		}
		if !json.Valid(request.JSONSchema.Definition) {
			return openai.ChatCompletionNewParams{}, errors.New("completion JSON schema is invalid JSON")
		}
		definition := append(json.RawMessage(nil), request.JSONSchema.Definition...)
		switch c.structuredOutput {
		case StructuredOutputJSONSchema:
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
					JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
						Name: name, Strict: param.NewOpt(true), Schema: definition,
					},
				},
			}
		case StructuredOutputJSONObject:
			if len(params.Messages) == 0 {
				return openai.ChatCompletionNewParams{}, errors.New(
					"completion JSON schema requires at least one message",
				)
			}
			instruction := "\n\nReturn only one JSON object that satisfies this JSON Schema. Do not use Markdown fences:\n" +
				string(request.JSONSchema.Definition)
			params.Messages = append(params.Messages, openai.UserMessage(instruction))
			format := shared.NewResponseFormatJSONObjectParam()
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: &format,
			}
		}
	default:
		return openai.ChatCompletionNewParams{}, fmt.Errorf(
			"completion response format: unsupported format %q",
			request.ResponseFormat,
		)
	}
	return params, nil
}
