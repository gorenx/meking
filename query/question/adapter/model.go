package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/agent"
	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/query/internal/modeladapter"
	queryquestion "github.com/memoria-space/meking/query/question"
)

type completion interface {
	Complete(context.Context, agent.CompletionRequest) (agent.CompletionResponse, error)
}

type streamingCompletion interface {
	completion
	Stream(context.Context, agent.CompletionRequest, agent.CompletionDeltaHandler) (agent.CompletionStreamResponse, error)
}

// CompletionModel maps the Question candidate contract to the shared model
// transport. It consumes streaming output internally so callers only observe a
// complete list that can be normalized atomically.
type CompletionModel struct {
	completion completion
}

var _ queryquestion.Model = (*CompletionModel)(nil)
var _ queryquestion.CandidateCorrector = (*CompletionModel)(nil)

func NewCompletionModel(completion completion) (*CompletionModel, error) {
	if completion == nil {
		return nil, errors.New("create Question CompletionModel: completion is required")
	}
	if _, ok := completion.(streamingCompletion); !ok {
		return nil, errors.New("create Question CompletionModel: streaming completion is required")
	}
	return &CompletionModel{completion: completion}, nil
}

func (m *CompletionModel) Generate(
	ctx context.Context,
	request queryquestion.ModelRequest,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Question CompletionModel is not configured"))
	}
	streamer, ok := m.completion.(streamingCompletion)
	if !ok {
		return "", querybase.NewInternalFailure(
			errors.New("Question completion model does not support streaming"),
		)
	}
	response, err := streamer.Stream(ctx, agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: request.SystemPrompt},
			{Role: agent.CompletionRoleUser, Content: request.UserPrompt},
		},
		ResponseFormat: agent.CompletionResponseText,
	}, func(string) error { return nil })
	if err != nil {
		return response.Content, modeladapter.Failure(err)
	}
	return response.Content, nil
}

func (m *CompletionModel) CorrectCandidates(
	ctx context.Context,
	correction queryquestion.CandidateCorrection,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Question CompletionModel is not configured"))
	}
	response, err := m.completion.Complete(ctx, agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: correction.Request.SystemPrompt},
			{Role: agent.CompletionRoleUser, Content: correction.Request.UserPrompt},
			{Role: agent.CompletionRoleAssistant, Content: correction.Result},
			{
				Role: agent.CompletionRoleUser,
				Content: "Your previous result was rejected.\n" +
					"Rejected result:\n" + correction.Result + "\n" +
					"Reason:\n" + correction.Reason + "\n" +
					"Return the complete corrected question list only.",
			},
		},
		ResponseFormat: agent.CompletionResponseText,
	})
	if err != nil {
		return "", modeladapter.Failure(err)
	}
	return response.Content, nil
}
