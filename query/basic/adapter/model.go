package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/agent"
	querybase "github.com/memoria-space/meking/query"
	querybasic "github.com/memoria-space/meking/query/basic"
	"github.com/memoria-space/meking/query/internal/modeladapter"
)

type completion interface {
	Complete(context.Context, agent.CompletionRequest) (agent.CompletionResponse, error)
}

type streamingCompletion interface {
	completion
	Stream(context.Context, agent.CompletionRequest, agent.CompletionDeltaHandler) (agent.CompletionStreamResponse, error)
}

// CompletionModel maps Basic's two-message contract onto the provider-neutral
// completion API. Prompt construction and Citation rules remain in Basic.
type CompletionModel struct {
	// completion is the selected provider-neutral model shared by sync and stream.
	completion completion
}

var (
	_ querybasic.AnswerModel       = (*CompletionModel)(nil)
	_ querybasic.StreamAnswerModel = (*CompletionModel)(nil)
)

// NewCompletionModel creates the Basic answer adapter.
func NewCompletionModel(completion completion) (*CompletionModel, error) {
	if completion == nil {
		return nil, errors.New("create Basic CompletionModel: completion is required")
	}
	return &CompletionModel{completion: completion}, nil
}

// GenerateAnswer requests one unconstrained text completion.
func (m *CompletionModel) GenerateAnswer(
	ctx context.Context,
	request querybasic.AnswerModelRequest,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Basic CompletionModel is not configured"))
	}
	response, err := m.completion.Complete(ctx, completionRequest(request))
	if err != nil {
		return "", modeladapter.Failure(err)
	}
	return response.Content, nil
}

// StreamAnswer emits only final Basic answer text and returns the exact emitted prefix.
func (m *CompletionModel) StreamAnswer(
	ctx context.Context,
	request querybasic.AnswerModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Basic CompletionModel is not configured"))
	}
	streamer, ok := m.completion.(streamingCompletion)
	if !ok {
		return "", querybase.NewInternalFailure(
			errors.New("Basic completion model does not support streaming"),
		)
	}
	response, err := streamer.Stream(
		ctx,
		completionRequest(request),
		agent.CompletionDeltaHandler(emit),
	)
	if err != nil {
		return response.Content, modeladapter.Failure(err)
	}
	return response.Content, nil
}

func completionRequest(request querybasic.AnswerModelRequest) agent.CompletionRequest {
	return agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: request.SystemPrompt},
			{Role: agent.CompletionRoleUser, Content: request.UserPrompt},
		},
		ResponseFormat: agent.CompletionResponseText,
	}
}
