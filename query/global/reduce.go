package global

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
)

const (
	// DefaultDataTokens is the Reduce budget for ranked Map conclusions.
	DefaultDataTokens = 12000
	// DefaultReduceLength is the requested word limit for the final answer.
	DefaultReduceLength = 2000
	// DefaultResponseType requests a multi-paragraph answer.
	DefaultResponseType = "Multiple Paragraphs"
	// NoDataAnswer is returned without a model call when no positive Map point
	// exists and general knowledge is disabled.
	NoDataAnswer = "I am sorry but I am unable to answer this question given the provided data."
)

// ReduceModelRequest is the provider-independent two-message input for the
// final answer after ranked Map conclusions fit the Reduce budget.
type ReduceModelRequest struct {
	// SystemPrompt contains accepted Analyst blocks and answer constraints.
	SystemPrompt string
	// UserPrompt is the original corpus-wide question.
	UserPrompt string
}

// ReduceModel generates one final answer from accepted Map conclusions.
type ReduceModel interface {
	GenerateReduce(ctx context.Context, request ReduceModelRequest) (string, error)
}

// StreamReduceModel emits only final Reduce text; Map results never become
// answer deltas.
type StreamReduceModel interface {
	StreamReduce(
		ctx context.Context,
		request ReduceModelRequest,
		emit querybase.TextDeltaHandler,
	) (string, error)
}

// ReduceConfig controls the Map-point budget and answer instructions. It does
// not change Report selection, Context construction, or Map scoring.
type ReduceConfig struct {
	// DataMaxTokens is the positive budget for formatted Map points only.
	DataMaxTokens int
	// MaxLength is the positive word-count instruction for the final answer.
	MaxLength int
	// ResponseType is the default answer format; empty selects DefaultResponseType.
	ResponseType string
	// AllowGeneralKnowledge permits a model call without positive Map points and
	// appends KnowledgePrompt to every Reduce system prompt.
	AllowGeneralKnowledge bool
}

// DefaultReduceConfig returns the built-in bounded Reduce policy.
func DefaultReduceConfig() ReduceConfig {
	return ReduceConfig{
		DataMaxTokens: DefaultDataTokens,
		MaxLength:     DefaultReduceLength,
		ResponseType:  DefaultResponseType,
	}
}

// Validate rejects request policies that cannot perform bounded Reduce work.
func (c ReduceConfig) Validate() error {
	if c.DataMaxTokens <= 0 {
		return errors.New("Global Reduce data token budget must be positive")
	}
	if c.MaxLength <= 0 {
		return errors.New("Global Reduce response length must be positive")
	}
	return nil
}

// ReduceContext records exactly which ranked conclusions entered the final
// prompt and whether the first over-budget point stopped traversal.
type ReduceContext struct {
	// Text is the Analyst-block text inserted into the final system prompt.
	Text string
	// TokenCount sums separately counted Analyst blocks; separators are excluded.
	TokenCount int
	// Points preserves accepted conclusions in stable descending-score order.
	Points []MapPoint
	// Truncated is true when a positive point was rejected by the budget.
	Truncated bool
}

// ReduceRequest combines the original question and one completed Map phase.
type ReduceRequest struct {
	// Question is sent unchanged as the final user message.
	Question string
	// Map supplies scored conclusions and the fixed ReportSet Context.
	Map MapResult
	// ResponseType overrides the configured answer format when non-empty.
	ResponseType string
}

// SearchResult is the Global answer plus the exact Map and Reduce audit trail.
// Citation is attached by the complete search use case after source resolution.
type SearchResult struct {
	// Response is the final model or canned no-data text and is never rewritten.
	Response string
	// Context is the fixed ReportSet evidence supplied to Map calls.
	Context Context
	// Map preserves every successful or isolated batch outcome.
	Map MapResult
	// ReduceContext records the ranked conclusions accepted by the final budget.
	ReduceContext ReduceContext
	// CitationAudit records whether generated Report references resolve to the
	// exact Corpora fixed by Context. It annotates but never rewrites Response.
	CitationAudit querybase.CitationAudit
}

// Reducer ranks Map conclusions, applies the Reduce-only budget, and asks a
// provider-neutral model for the final answer.
type Reducer struct {
	model           ReduceModel
	tokens          TokenCounter
	prompt          string
	knowledgePrompt string
	config          ReduceConfig
}

// NewReducer creates a reusable final-answer reducer.
func NewReducer(
	model ReduceModel,
	tokens TokenCounter,
	prompt string,
	knowledgePrompt string,
	config ReduceConfig,
) (*Reducer, error) {
	if model == nil {
		return nil, errors.New("create Global Reducer: ReduceModel is required")
	}
	if tokens == nil {
		return nil, errors.New("create Global Reducer: TokenCounter is required")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("create Global Reducer: Reduce prompt is required")
	}
	if config.AllowGeneralKnowledge && strings.TrimSpace(knowledgePrompt) == "" {
		return nil, errors.New("create Global Reducer: knowledge prompt is required when enabled")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Reducer{
		model: model, tokens: tokens, prompt: prompt,
		knowledgePrompt: knowledgePrompt, config: config,
	}, nil
}

// Reduce produces a final answer without rebuilding or mutating Map evidence.
func (r *Reducer) Reduce(ctx context.Context, request ReduceRequest) (SearchResult, error) {
	result, modelRequest, callModel, err := r.prepare(ctx, request)
	if err != nil || !callModel {
		return result, err
	}
	response, err := r.model.GenerateReduce(ctx, modelRequest)
	if err != nil {
		return SearchResult{}, fmt.Errorf("generate Global Reduce answer: %w", err)
	}
	result.Response = response
	return result, nil
}

// Stream emits only final Reduce text and retains any partial response when
// the provider, context, or downstream consumer terminates the stream.
func (r *Reducer) Stream(
	ctx context.Context,
	request ReduceRequest,
	emit querybase.TextDeltaHandler,
) (SearchResult, error) {
	if r == nil || r.model == nil {
		return SearchResult{}, errors.New("Global Reducer is not configured")
	}
	streamModel, ok := r.model.(StreamReduceModel)
	if !ok {
		return SearchResult{}, errors.New("Global Reduce model does not support streaming")
	}
	if emit == nil {
		return SearchResult{}, errors.New("Global Reduce stream handler is required")
	}
	result, modelRequest, callModel, err := r.prepare(ctx, request)
	if err != nil {
		return result, err
	}
	if !callModel {
		if result.Response != "" {
			if err := emit(result.Response); err != nil {
				return result, err
			}
		}
		return result, nil
	}
	response, err := streamModel.StreamReduce(ctx, modelRequest, emit)
	result.Response = response
	if err != nil {
		return result, fmt.Errorf("stream Global Reduce answer: %w", err)
	}
	return result, nil
}

func (r *Reducer) prepare(
	ctx context.Context,
	request ReduceRequest,
) (SearchResult, ReduceModelRequest, bool, error) {
	if r == nil || r.model == nil || r.tokens == nil {
		return SearchResult{}, ReduceModelRequest{}, false,
			errors.New("Global Reducer is not configured")
	}
	if err := ctx.Err(); err != nil {
		return SearchResult{}, ReduceModelRequest{}, false, err
	}

	result := SearchResult{Context: request.Map.Context, Map: request.Map}
	points := request.Map.RankedPoints()
	if len(points) == 0 && !r.config.AllowGeneralKnowledge {
		result.Response = NoDataAnswer
		return result, ReduceModelRequest{}, false, nil
	}

	blocks := make([]string, 0, len(points))
	for _, point := range points {
		block := formatReducePoint(point)
		count, err := r.tokens.Count(ctx, request.Map.Context.CorporaID, block)
		if err != nil {
			return SearchResult{}, ReduceModelRequest{}, false,
				fmt.Errorf("count Global Reduce point tokens: %w", err)
		}
		if result.ReduceContext.TokenCount+count > r.config.DataMaxTokens {
			result.ReduceContext.Truncated = true
			break
		}
		blocks = append(blocks, block)
		result.ReduceContext.TokenCount += count
		result.ReduceContext.Points = append(result.ReduceContext.Points, point)
	}
	result.ReduceContext.Text = strings.Join(blocks, "\n\n")

	responseType := strings.TrimSpace(request.ResponseType)
	if responseType == "" {
		responseType = strings.TrimSpace(r.config.ResponseType)
	}
	if responseType == "" {
		responseType = DefaultResponseType
	}
	systemPrompt, err := queryprompt.Render(r.prompt, map[string]string{
		"report_data":   result.ReduceContext.Text,
		"response_type": responseType,
		"max_length":    strconv.Itoa(r.config.MaxLength),
	})
	if err != nil {
		return SearchResult{}, ReduceModelRequest{}, false,
			fmt.Errorf("render Global Reduce prompt: %w", err)
	}
	if r.config.AllowGeneralKnowledge {
		systemPrompt += "\n" + r.knowledgePrompt
	}
	return result, ReduceModelRequest{SystemPrompt: systemPrompt, UserPrompt: request.Question}, true, nil
}

func formatReducePoint(point MapPoint) string {
	return fmt.Sprintf(
		"----Analyst %d----\nImportance Score: %d\n%s",
		point.BatchIndex+1,
		point.Score,
		point.Description,
	)
}
