package question

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
)

// Generator completes the Question Generation use case: it validates history,
// obtains one fixed Local context, invokes the candidate model once, and turns
// the model's line list into a stable bounded result.
type Generator struct {
	evidence EvidenceProvider
	model    Model
	tokens   querybase.TokenCounter
	prompt   string
}

func NewGenerator(
	evidence EvidenceProvider,
	model Model,
	tokens querybase.TokenCounter,
	prompt string,
) (*Generator, error) {
	if evidence == nil {
		return nil, errors.New("create Question Generator: evidence provider is required")
	}
	if model == nil {
		return nil, errors.New("create Question Generator: model is required")
	}
	if tokens == nil {
		return nil, errors.New("create Question Generator: token counter is required")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("create Question Generator: prompt is required")
	}
	for _, field := range []string{"{context_data}", "{question_count}"} {
		if !strings.Contains(prompt, field) {
			return nil, fmt.Errorf("create Question Generator: prompt is missing %s", field)
		}
	}
	return &Generator{evidence: evidence, model: model, tokens: tokens, prompt: prompt}, nil
}

// Suggest returns at most Request.Count first-occurrence candidates. Invalid
// input fails before Local retrieval; empty normalized model output is a typed
// invalid-model result rather than a successful empty suggestion list.
func (g *Generator) Suggest(ctx context.Context, request Request) (Result, error) {
	if g == nil || g.evidence == nil || g.model == nil || g.tokens == nil {
		return Result{}, querybase.NewInternalFailure(errors.New("Question Generator is not configured"))
	}
	if err := ValidateRequest(request); err != nil {
		return Result{}, querybase.NewInvalidInputFailure(err.Error(), err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, querybase.NewCancelledFailure(err)
	}
	current := request.History[len(request.History)-1]
	evidence, err := g.evidence.Prepare(ctx, EvidenceRequest{
		CurrentQuestion:   current,
		PreviousQuestions: append([]string(nil), request.History[:len(request.History)-1]...),
	})
	result := Result{Evidence: evidence}
	if err != nil {
		return result, normalizeFailure(err)
	}
	systemPrompt, err := queryprompt.Render(g.prompt, map[string]string{
		"context_data": evidence.Context, "question_count": strconv.Itoa(request.Count),
	})
	if err != nil {
		return result, querybase.NewInternalFailure(fmt.Errorf("render Question prompt: %w", err))
	}
	result.PromptTokens, err = g.tokens.Count(ctx, evidence.CorporaID, systemPrompt)
	if err != nil {
		return result, normalizeFailure(fmt.Errorf("count Question prompt tokens: %w", err))
	}
	modelRequest := ModelRequest{SystemPrompt: systemPrompt, UserPrompt: current}
	result.ModelCallCount = 1
	raw, err := g.model.Generate(ctx, modelRequest)
	if err != nil {
		return result, normalizeFailure(err)
	}
	result.Questions = normalizeCandidates(raw, request.Count)
	if len(result.Questions) == 0 {
		reason := errors.New("Question model returned no non-empty candidates")
		if corrector, supported := g.model.(CandidateCorrector); supported {
			result.ModelCallCount++
			raw, err = corrector.CorrectCandidates(ctx, CandidateCorrection{
				Request: modelRequest,
				Result:  raw,
				Reason:  reason.Error(),
			})
			if err != nil {
				return result, normalizeFailure(err)
			}
			result.Questions = normalizeCandidates(raw, request.Count)
		}
		if len(result.Questions) == 0 {
			return result, querybase.NewInvalidModelResponseFailure(reason)
		}
	}
	return result, nil
}

func ValidateRequest(request Request) error {
	if request.Count <= 0 || request.Count > MaximumCount {
		return fmt.Errorf("question count must be between 1 and %d", MaximumCount)
	}
	if len(request.History) == 0 {
		return errors.New("at least one question is required")
	}
	for index, value := range request.History {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("question history item %d must not be empty", index)
		}
	}
	return nil
}

func normalizeCandidates(raw string, limit int) []string {
	result := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, line := range strings.Split(raw, "\n") {
		candidate := strings.TrimSpace(line)
		if strings.HasPrefix(candidate, "-") {
			candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "-"))
		}
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
		if len(result) == limit {
			break
		}
	}
	return result
}

func normalizeFailure(err error) error {
	if err == nil {
		return nil
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	return querybase.NewInternalFailure(err)
}
