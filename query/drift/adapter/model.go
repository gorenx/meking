package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/agent"
	querybase "github.com/memoria-space/meking/query"
	querydrift "github.com/memoria-space/meking/query/drift"
	"github.com/memoria-space/meking/query/internal/modeladapter"
)

type completion interface {
	Complete(context.Context, agent.CompletionRequest) (agent.CompletionResponse, error)
}

type streamingCompletion interface {
	completion
	Stream(context.Context, agent.CompletionRequest, agent.CompletionDeltaHandler) (agent.CompletionStreamResponse, error)
}

// CompletionModel maps DRIFT's text hypothetical and JSON Primer operations to
// the provider-neutral completion API. Prompt construction and response schema
// validation remain in the DRIFT aggregate.
type CompletionModel struct {
	completion completion
}

var (
	_ querydrift.Model             = (*CompletionModel)(nil)
	_ querydrift.PrimerCorrector   = (*CompletionModel)(nil)
	_ querydrift.BranchModel       = (*CompletionModel)(nil)
	_ querydrift.BranchCorrector   = (*CompletionModel)(nil)
	_ querydrift.ReduceModel       = (*CompletionModel)(nil)
	_ querydrift.StreamReduceModel = (*CompletionModel)(nil)
)

// NewCompletionModel creates the DRIFT model adapter.
func NewCompletionModel(completion completion) (*CompletionModel, error) {
	if completion == nil {
		return nil, errors.New("create DRIFT CompletionModel: completion is required")
	}
	return &CompletionModel{completion: completion}, nil
}

// GenerateHypothetical requests unconstrained text for vector retrieval.
func (m *CompletionModel) GenerateHypothetical(
	ctx context.Context,
	request querydrift.ModelRequest,
) (string, error) {
	return m.complete(ctx, request, agent.CompletionResponseText)
}

// GeneratePrimer requests one JSON object containing the fold answer, score,
// and follow-up questions.
func (m *CompletionModel) GeneratePrimer(
	ctx context.Context,
	request querydrift.ModelRequest,
) (string, error) {
	return m.complete(ctx, request, agent.CompletionResponseJSONObject)
}

func (m *CompletionModel) CorrectPrimer(
	ctx context.Context,
	correction querydrift.PrimerCorrection,
) (string, error) {
	return m.correct(ctx, resultCorrection{
		messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleUser, Content: correction.Request.Prompt},
			{Role: agent.CompletionRoleAssistant, Content: correction.Result},
			{Role: agent.CompletionRoleUser, Content: correction.Feedback()},
		},
		maxCompletionTokens: correction.Request.MaxCompletionTokens,
	})
}

// GenerateBranch sends the exact Local evidence as the system message and the
// branch question as the user message, requesting the DRIFT JSON schema.
func (m *CompletionModel) GenerateBranch(
	ctx context.Context,
	request querydrift.BranchModelRequest,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(
			errors.New("DRIFT CompletionModel is not configured"),
		)
	}
	response, err := m.completion.Complete(ctx, agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: request.SystemPrompt},
			{Role: agent.CompletionRoleUser, Content: request.UserPrompt},
		},
		ResponseFormat:      agent.CompletionResponseJSONObject,
		MaxCompletionTokens: request.MaxCompletionTokens,
	})
	if err != nil {
		return "", modeladapter.Failure(err)
	}
	return response.Content, nil
}

func (m *CompletionModel) CorrectBranch(
	ctx context.Context,
	correction querydrift.BranchCorrection,
) (string, error) {
	return m.correct(ctx, resultCorrection{
		messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: correction.Request.SystemPrompt},
			{Role: agent.CompletionRoleUser, Content: correction.Request.UserPrompt},
			{Role: agent.CompletionRoleAssistant, Content: correction.Result},
			{Role: agent.CompletionRoleUser, Content: correction.Feedback()},
		},
		maxCompletionTokens: correction.Request.MaxCompletionTokens,
	})
}

type resultCorrection struct {
	messages            []agent.CompletionMessage
	maxCompletionTokens int
}

func (m *CompletionModel) correct(ctx context.Context, correction resultCorrection) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(
			errors.New("DRIFT CompletionModel is not configured"),
		)
	}
	response, err := m.completion.Complete(ctx, agent.CompletionRequest{
		Messages:            append([]agent.CompletionMessage(nil), correction.messages...),
		ResponseFormat:      agent.CompletionResponseJSONObject,
		MaxCompletionTokens: correction.maxCompletionTokens,
	})
	if err != nil {
		return "", modeladapter.Failure(err)
	}
	return response.Content, nil
}

// GenerateReduce requests the final unconstrained DRIFT answer from the
// request-global, citation-renumbered context.
func (m *CompletionModel) GenerateReduce(
	ctx context.Context,
	request querydrift.ReduceModelRequest,
) (string, error) {
	return m.reduce(ctx, request, nil)
}

// StreamReduce emits only the final DRIFT answer and returns the exact emitted
// prefix when the provider or handler terminates the stream.
func (m *CompletionModel) StreamReduce(
	ctx context.Context,
	request querydrift.ReduceModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	return m.reduce(ctx, request, emit)
}

func (m *CompletionModel) reduce(
	ctx context.Context,
	request querydrift.ReduceModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(
			errors.New("DRIFT CompletionModel is not configured"),
		)
	}
	completionRequest := agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: request.SystemPrompt},
			{Role: agent.CompletionRoleUser, Content: request.UserPrompt},
		},
		ResponseFormat:      agent.CompletionResponseText,
		MaxCompletionTokens: request.MaxCompletionTokens,
	}
	if emit == nil {
		response, err := m.completion.Complete(ctx, completionRequest)
		if err != nil {
			return "", modeladapter.Failure(err)
		}
		return response.Content, nil
	}
	streamer, ok := m.completion.(streamingCompletion)
	if !ok {
		return "", querybase.NewInternalFailure(
			errors.New("DRIFT completion model does not support streaming"),
		)
	}
	response, err := streamer.Stream(ctx, completionRequest, agent.CompletionDeltaHandler(emit))
	if err != nil {
		return response.Content, modeladapter.Failure(err)
	}
	return response.Content, nil
}

func (m *CompletionModel) complete(
	ctx context.Context,
	request querydrift.ModelRequest,
	format agent.CompletionResponseFormat,
) (string, error) {
	if m == nil || m.completion == nil {
		return "", querybase.NewInternalFailure(
			errors.New("DRIFT CompletionModel is not configured"),
		)
	}
	response, err := m.completion.Complete(ctx, agent.CompletionRequest{
		Messages: []agent.CompletionMessage{{
			Role: agent.CompletionRoleUser, Content: request.Prompt,
		}},
		ResponseFormat:      format,
		MaxCompletionTokens: request.MaxCompletionTokens,
	})
	if err != nil {
		return "", modeladapter.Failure(err)
	}
	return response.Content, nil
}
