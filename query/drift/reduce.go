package drift

import (
	"context"
	"errors"
	"fmt"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
)

type ReduceModelRequest struct {
	SystemPrompt        string
	UserPrompt          string
	MaxCompletionTokens int
}

type ReduceModel interface {
	GenerateReduce(ctx context.Context, request ReduceModelRequest) (string, error)
}

type StreamReduceModel interface {
	StreamReduce(
		ctx context.Context,
		request ReduceModelRequest,
		emit querybase.TextDeltaHandler,
	) (string, error)
}

// IntermediateAnswer is one complete Primer fold or successful Local branch
// admitted to final Reduce. Response is a rewritten copy using only IDs from
// ReduceContext.CitationRecords; the original model answer remains unchanged.
type IntermediateAnswer struct {
	Question string
	Response string
	Score    float64
}

// ReduceContext is the exact context_data sent to the final model. Answers are
// admitted as whole blocks in order; CitationRecords is the only final scope.
type ReduceContext struct {
	Text            string
	TokenCount      int
	Answers         []IntermediateAnswer
	CitationRecords []querybase.CitationRecord
	Truncated       bool
}

type ReduceRequest struct {
	Traversal    TraversalResult
	ResponseType string
}

// Reduction is the final model stage output. The DRIFT Searcher retains the
// Traversal and performs Citation audit after this stage succeeds.
type Reduction struct {
	Response string
	Context  ReduceContext
	Usage    Usage
}

// Reducer admits stable intermediate answers, assigns request-global Citation
// IDs, and performs the mandatory final model call within cumulative limits.
type Reducer struct {
	model  ReduceModel
	tokens querybase.TokenCounter
	prompt string
	config ReduceConfig
	limits RequestLimits
}

func NewReducer(
	model ReduceModel,
	tokens querybase.TokenCounter,
	prompt string,
	config ReduceConfig,
	limits RequestLimits,
) (*Reducer, error) {
	switch {
	case model == nil:
		return nil, errors.New("create DRIFT Reducer: ReduceModel is required")
	case tokens == nil:
		return nil, errors.New("create DRIFT Reducer: TokenCounter is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if err := validatePrompt(
		prompt,
		map[string]string{
			"context_data": "\x00context\x00", "response_type": "\x00response\x00",
		},
		[]string{"\x00context\x00", "\x00response\x00"},
	); err != nil {
		return nil, fmt.Errorf("validate DRIFT Reduce prompt: %w", err)
	}
	return &Reducer{model: model, tokens: tokens, prompt: prompt, config: config, limits: limits}, nil
}

func (r *Reducer) Reduce(ctx context.Context, request ReduceRequest) (Reduction, error) {
	return r.execute(ctx, request, func(
		callContext context.Context,
		modelRequest ReduceModelRequest,
	) (string, error) {
		return r.model.GenerateReduce(callContext, modelRequest)
	}, nil)
}

func (r *Reducer) reduce(
	ctx context.Context,
	request ReduceRequest,
	budget *requestBudget,
) (Reduction, error) {
	return r.execute(ctx, request, func(
		callContext context.Context,
		modelRequest ReduceModelRequest,
	) (string, error) {
		return r.model.GenerateReduce(callContext, modelRequest)
	}, budget)
}

func (r *Reducer) Stream(
	ctx context.Context,
	request ReduceRequest,
	emit querybase.TextDeltaHandler,
) (Reduction, error) {
	return r.stream(ctx, request, emit, nil)
}

func (r *Reducer) stream(
	ctx context.Context,
	request ReduceRequest,
	emit querybase.TextDeltaHandler,
	budget *requestBudget,
) (Reduction, error) {
	if r == nil || r.model == nil {
		return Reduction{}, querybase.NewInternalFailure(errors.New("DRIFT Reducer is not configured"))
	}
	streamer, ok := r.model.(StreamReduceModel)
	if !ok {
		return Reduction{}, querybase.NewInternalFailure(
			errors.New("DRIFT Reduce model does not support streaming"),
		)
	}
	if emit == nil {
		return Reduction{}, querybase.NewInvalidInputFailure(
			"DRIFT Reduce stream handler is required", nil,
		)
	}
	result, err := r.execute(ctx, request, func(
		callContext context.Context,
		modelRequest ReduceModelRequest,
	) (string, error) {
		return streamer.StreamReduce(callContext, modelRequest, emit)
	}, budget)
	if err != nil && result.Response != "" {
		return result, querybase.MarkPartialOutput(err)
	}
	return result, err
}

type reduceCall func(context.Context, ReduceModelRequest) (string, error)

func (r *Reducer) execute(
	ctx context.Context,
	request ReduceRequest,
	call reduceCall,
	budget *requestBudget,
) (Reduction, error) {
	if r == nil || r.model == nil || r.tokens == nil {
		return Reduction{},
			querybase.NewInternalFailure(errors.New("DRIFT Reducer is not configured"))
	}
	if strings.TrimSpace(request.Traversal.Primer.Question) == "" {
		return Reduction{},
			querybase.NewInvalidInputFailure("DRIFT Reduce question is required", nil)
	}
	responseType, err := r.responseType(request.ResponseType)
	if err != nil {
		return Reduction{}, err
	}
	if err = ctx.Err(); err != nil {
		return Reduction{}, normalizeFailure(err)
	}
	if budget == nil {
		budget = newRequestBudget(r.limits, request.Traversal.Usage)
		if !budget.hold(1, r.config.MaxCompletionTokens) {
			return Reduction{}, querybase.NewInvalidInputFailure(
				"the DRIFT traversal leaves no model-call or output-token capacity for final Reduce",
				nil,
			)
		}
	}
	contextData, err := r.buildContext(
		ctx,
		request.Traversal,
		responseType,
		budget.remainingPromptTokens(),
	)
	if err != nil {
		return Reduction{Context: contextData, Usage: request.Traversal.Usage}, err
	}
	result := Reduction{Context: contextData, Usage: request.Traversal.Usage}

	if len(contextData.Answers) == 0 {
		return result, querybase.NewNoEvidenceFailure(
			errors.New("DRIFT has no intermediate answer for final Reduce"),
		)
	}
	modelRequest, err := r.modelRequest(request.Traversal.Primer.Question, responseType, contextData)
	if err != nil {
		return result, err
	}
	promptTokens, err := r.countPrompt(
		ctx,
		request.Traversal.Primer.Epoch.CorporaID,
		modelRequest.SystemPrompt,
		modelRequest.UserPrompt,
	)
	if err != nil {
		return result, err
	}
	if !budget.useHeld(promptTokens, r.config.MaxCompletionTokens) {
		return result, querybase.NewInternalFailure(
			errors.New("DRIFT Reduce lost its reserved request capacity"),
		)
	}
	result.Usage = budget.usage

	response, modelErr := call(ctx, modelRequest)
	result.Response = response
	if modelErr != nil {
		outputTokens, _ := r.countOutput(
			ctx, request.Traversal.Primer.Epoch.CorporaID, response,
		)
		budget.finish(r.config.MaxCompletionTokens, outputTokens)
		result.Usage = budget.usage
		return result, normalizeFailure(modelErr)
	}
	outputTokens, outputErr := r.countOutput(
		ctx, request.Traversal.Primer.Epoch.CorporaID, response,
	)
	withinCompletionLimit := budget.finish(r.config.MaxCompletionTokens, outputTokens)
	result.Usage = budget.usage
	if outputErr != nil {
		return result, outputErr
	}
	if !withinCompletionLimit || budget.usage.OutputTokens > budget.limits.OutputTokens {
		return result, querybase.NewInvalidModelResponseFailure(fmt.Errorf(
			"DRIFT Reduce output contains %d tokens beyond its reserved limits",
			outputTokens,
		))
	}
	return result, nil
}

func (r *Reducer) responseType(value string) (string, error) {
	if value == "" {
		return r.config.ResponseType, nil
	}
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return "", querybase.NewInvalidInputFailure(
			"DRIFT response type must not have surrounding whitespace", nil,
		)
	}
	return value, nil
}

func (r *Reducer) modelRequest(
	question string,
	responseType string,
	contextData ReduceContext,
) (ReduceModelRequest, error) {
	systemPrompt, err := queryprompt.Render(r.prompt, map[string]string{
		"context_data": contextData.Text, "response_type": responseType,
	})
	if err != nil {
		return ReduceModelRequest{}, querybase.NewInternalFailure(
			fmt.Errorf("render DRIFT Reduce prompt: %w", err),
		)
	}
	return ReduceModelRequest{
		SystemPrompt: systemPrompt, UserPrompt: question,
		MaxCompletionTokens: r.config.MaxCompletionTokens,
	}, nil
}

func (r *Reducer) countPrompt(
	ctx context.Context,
	corporaID string,
	systemPrompt string,
	userPrompt string,
) (int, error) {
	systemTokens, err := r.tokens.Count(ctx, corporaID, systemPrompt)
	if err != nil {
		return 0, normalizeFailure(err)
	}
	userTokens, err := r.tokens.Count(ctx, corporaID, userPrompt)
	if err != nil {
		return 0, normalizeFailure(err)
	}
	if systemTokens < 0 || userTokens < 0 {
		return 0, querybase.NewInternalFailure(
			errors.New("DRIFT TokenCounter returned a negative Reduce prompt count"),
		)
	}
	return systemTokens + userTokens, nil
}

func (r *Reducer) countOutput(
	ctx context.Context,
	corporaID string,
	response string,
) (int, error) {
	outputTokens, err := r.tokens.Count(ctx, corporaID, response)
	if err != nil {
		return 0, normalizeFailure(err)
	}
	if outputTokens < 0 {
		return 0, querybase.NewInternalFailure(
			errors.New("DRIFT TokenCounter returned a negative Reduce output count"),
		)
	}
	return outputTokens, nil
}
