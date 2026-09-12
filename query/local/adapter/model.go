package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/agent"
	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/query/internal/modeladapter"
	querylocal "github.com/memoria-space/meking/query/local"
)

type completion interface {
	Complete(context.Context, agent.CompletionRequest) (agent.CompletionResponse, error)
}

type streamingCompletion interface {
	completion
	Stream(context.Context, agent.CompletionRequest, agent.CompletionDeltaHandler) (agent.CompletionStreamResponse, error)
}

// CompletionModel maps Local's answer contract onto the provider-neutral
// conversational completion API. Prompt construction and Citation remain in
// the Local aggregate.
type CompletionModel struct {
	// completion executes complete or streaming text completion requests.
	completion completion
}

var (
	_ querylocal.AnswerModel       = (*CompletionModel)(nil)
	_ querylocal.StreamAnswerModel = (*CompletionModel)(nil)
)

// NewCompletionModel creates the Local answer adapter.
func NewCompletionModel(completion completion) (*CompletionModel, error) {
	if completion == nil {
		return nil, errors.New("create Local CompletionModel: completion is required")
	}
	return &CompletionModel{completion: completion}, nil
}

// GenerateAnswer sends Local's exact system and user messages as text output.
func (m *CompletionModel) GenerateAnswer(
	ctx context.Context,
	request querylocal.AnswerModelRequest,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Local CompletionModel is not configured"))
	}
	response, err := m.completion.Complete(ctx, completionRequest(request))
	if err != nil {
		return "", modeladapter.Failure(err)
	}
	return response.Content, nil
}

// StreamAnswer emits only final Local answer text and returns the exact emitted prefix.
func (m *CompletionModel) StreamAnswer(
	ctx context.Context,
	request querylocal.AnswerModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Local CompletionModel is not configured"))
	}
	streamer, ok := m.completion.(streamingCompletion)
	if !ok {
		return "", querybase.NewInternalFailure(
			errors.New("Local completion model does not support streaming"),
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

func completionRequest(request querylocal.AnswerModelRequest) agent.CompletionRequest {
	return agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: request.SystemPrompt},
			{Role: agent.CompletionRoleUser, Content: request.UserPrompt},
		},
		ResponseFormat: agent.CompletionResponseText,
	}
}
