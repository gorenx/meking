package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/agent"
	querybase "github.com/memoria-space/meking/query"
	queryglobal "github.com/memoria-space/meking/query/global"
	"github.com/memoria-space/meking/query/internal/modeladapter"
)

type completion interface {
	Complete(context.Context, agent.CompletionRequest) (agent.CompletionResponse, error)
}

type streamingCompletion interface {
	completion
	Stream(context.Context, agent.CompletionRequest, agent.CompletionDeltaHandler) (agent.CompletionStreamResponse, error)
}

// CompletionModel translates Global Search's Map, rating, and Reduce ports to
// the provider-neutral conversational completion API. It owns message and
// response-format mapping but no prompt, selection, or ranking behavior.
type CompletionModel struct {
	// completion executes provider-neutral text or JSON-object requests.
	completion completion
}

var (
	_ queryglobal.MapModel          = (*CompletionModel)(nil)
	_ queryglobal.MapCorrector      = (*CompletionModel)(nil)
	_ queryglobal.RatingModel       = (*CompletionModel)(nil)
	_ queryglobal.RatingCorrector   = (*CompletionModel)(nil)
	_ queryglobal.ReduceModel       = (*CompletionModel)(nil)
	_ queryglobal.StreamReduceModel = (*CompletionModel)(nil)
)

// NewCompletionModel creates the Global Search model adapter.
func NewCompletionModel(completion completion) (*CompletionModel, error) {
	if completion == nil {
		return nil, errors.New("create Global CompletionModel: completion is required")
	}
	return &CompletionModel{completion: completion}, nil
}

// GenerateMap requests one structured Map result.
func (m *CompletionModel) GenerateMap(
	ctx context.Context,
	request queryglobal.MapModelRequest,
) (string, error) {
	return m.complete(ctx, request.SystemPrompt, request.UserPrompt, agent.CompletionResponseJSONObject)
}

func (m *CompletionModel) CorrectMap(
	ctx context.Context,
	correction queryglobal.MapCorrection,
) (string, error) {
	return m.correct(ctx, resultCorrection{
		systemPrompt: correction.Request.SystemPrompt,
		userPrompt:   correction.Request.UserPrompt,
		result:       correction.Result,
		reason:       correction.Reason,
		format:       agent.CompletionResponseJSONObject,
	})
}

// RateCommunity requests one structured hierarchy relevance rating.
func (m *CompletionModel) RateCommunity(
	ctx context.Context,
	request queryglobal.RatingRequest,
) (string, error) {
	return m.complete(ctx, request.SystemPrompt, request.UserPrompt, agent.CompletionResponseJSONObject)
}

func (m *CompletionModel) CorrectRating(
	ctx context.Context,
	correction queryglobal.RatingCorrection,
) (string, error) {
	return m.correct(ctx, resultCorrection{
		systemPrompt: correction.Request.SystemPrompt,
		userPrompt:   correction.Request.UserPrompt,
		result:       correction.Result,
		reason:       correction.Reason,
		format:       agent.CompletionResponseJSONObject,
	})
}

// GenerateReduce requests the final unconstrained text answer.
func (m *CompletionModel) GenerateReduce(
	ctx context.Context,
	request queryglobal.ReduceModelRequest,
) (string, error) {
	return m.complete(ctx, request.SystemPrompt, request.UserPrompt, agent.CompletionResponseText)
}

// StreamReduce emits only final Reduce text through the completion model's
// optional streaming contract.
func (m *CompletionModel) StreamReduce(
	ctx context.Context,
	request queryglobal.ReduceModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Global CompletionModel is not configured"))
	}
	streamer, ok := m.completion.(streamingCompletion)
	if !ok {
		return "", querybase.NewInternalFailure(
			errors.New("Global completion model does not support streaming"),
		)
	}
	response, err := streamer.Stream(ctx, completionRequest(
		request.SystemPrompt,
		request.UserPrompt,
		agent.CompletionResponseText,
	), agent.CompletionDeltaHandler(emit))
	if err != nil {
		return response.Content, modeladapter.Failure(err)
	}
	return response.Content, nil
}

func (m *CompletionModel) complete(
	ctx context.Context,
	systemPrompt string,
	userPrompt string,
	format agent.CompletionResponseFormat,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Global CompletionModel is not configured"))
	}
	response, err := m.completion.Complete(ctx, completionRequest(systemPrompt, userPrompt, format))
	if err != nil {
		return "", modeladapter.Failure(err)
	}
	return response.Content, nil
}

func completionRequest(
	systemPrompt string,
	userPrompt string,
	format agent.CompletionResponseFormat,
) agent.CompletionRequest {
	return agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: systemPrompt},
			{Role: agent.CompletionRoleUser, Content: userPrompt},
		},
		ResponseFormat: format,
	}
}

type resultCorrection struct {
	systemPrompt string
	userPrompt   string
	result       string
	reason       string
	format       agent.CompletionResponseFormat
}

func (m *CompletionModel) correct(ctx context.Context, correction resultCorrection) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(errors.New("Global CompletionModel is not configured"))
	}
	response, err := m.completion.Complete(ctx, agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: correction.systemPrompt},
			{Role: agent.CompletionRoleUser, Content: correction.userPrompt},
			{Role: agent.CompletionRoleAssistant, Content: correction.result},
			{
				Role: agent.CompletionRoleUser,
				Content: "Your previous result was rejected.\n" +
					"Rejected result:\n" + correction.result + "\n" +
					"Reason:\n" + correction.reason + "\n" +
					"Return the complete corrected result in the required format only.",
			},
		},
		ResponseFormat: correction.format,
	})
	if err != nil {
		return "", modeladapter.Failure(err)
	}
	return response.Content, nil
}
