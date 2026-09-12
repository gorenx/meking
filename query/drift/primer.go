package drift

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
)

// Primer executes DRIFT's initial Epoch-fixed report retrieval and structured
// decomposition. It does not open Local branches, mutate a traversal graph, or
// produce the final user answer.
type Primer struct {
	epochs   querybase.EpochReader
	reports  ReportReader
	vectors  ReportVectorStore
	embedder Embedder
	model    Model
	tokens   querybase.TokenCounter
	// hypotheticalPrompt accepts only query and template fields.
	hypotheticalPrompt string
	// primerPrompt accepts only query and community_reports fields.
	primerPrompt string
	config       PrimerConfig
}

// NewPrimer creates the provider-independent initial DRIFT use case. Prompt
// templates are supplied by composition so Project prompt ownership does not
// leak into Query.
func NewPrimer(
	epochs querybase.EpochReader,
	reports ReportReader,
	vectors ReportVectorStore,
	embedder Embedder,
	model Model,
	tokens querybase.TokenCounter,
	hypotheticalPrompt string,
	primerPrompt string,
	config PrimerConfig,
) (*Primer, error) {
	switch {
	case epochs == nil:
		return nil, errors.New("create DRIFT Primer: Epoch reader is required")
	case reports == nil:
		return nil, errors.New("create DRIFT Primer: Report reader is required")
	case vectors == nil:
		return nil, errors.New("create DRIFT Primer: Report vector store is required")
	case embedder == nil || strings.TrimSpace(embedder.Model()) == "":
		return nil, errors.New("create DRIFT Primer: Embedder with a model is required")
	case model == nil:
		return nil, errors.New("create DRIFT Primer: Model is required")
	case tokens == nil:
		return nil, errors.New("create DRIFT Primer: TokenCounter is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := validatePrompt(
		hypotheticalPrompt,
		map[string]string{"query": "\x00query\x00", "template": "\x00template\x00"},
		[]string{"\x00query\x00", "\x00template\x00"},
	); err != nil {
		return nil, fmt.Errorf("validate DRIFT hypothetical prompt: %w", err)
	}
	if err := validatePrompt(
		primerPrompt,
		map[string]string{
			"query": "\x00query\x00", "community_reports": "\x00reports\x00",
		},
		[]string{"\x00query\x00", "\x00reports\x00"},
	); err != nil {
		return nil, fmt.Errorf("validate DRIFT Primer prompt: %w", err)
	}
	return &Primer{
		epochs: epochs, reports: reports, vectors: vectors, embedder: embedder,
		model: model, tokens: tokens, hypotheticalPrompt: hypotheticalPrompt,
		primerPrompt: primerPrompt, config: config,
	}, nil
}

// Search fixes Current Epoch once, retrieves its Top-K Reports, and analyzes
// every non-empty deterministic fold. A failure in any initial model call
// fails the whole Primer because later traversal requires a complete root.
func (p *Primer) Search(
	ctx context.Context,
	request PrimerRequest,
) (PrimerResult, error) {
	return p.search(ctx, request, nil)
}

func (p *Primer) search(
	ctx context.Context,
	request PrimerRequest,
	budget *requestBudget,
) (PrimerResult, error) {
	if p == nil || p.epochs == nil || p.reports == nil || p.vectors == nil ||
		p.embedder == nil || p.model == nil || p.tokens == nil {
		return PrimerResult{}, querybase.NewInternalFailure(
			errors.New("DRIFT Primer is not configured"),
		)
	}
	if strings.TrimSpace(request.Question) == "" {
		return PrimerResult{}, querybase.NewInvalidInputFailure(
			"DRIFT question is required", nil,
		)
	}
	if err := ctx.Err(); err != nil {
		return PrimerResult{}, normalizeFailure(err)
	}
	selected, err := p.epochs.Current(ctx)
	if errors.Is(err, querybase.ErrNoEpoch) {
		return PrimerResult{}, querybase.NewNoPublicationFailure(err)
	}
	if err != nil {
		return PrimerResult{}, normalizeFailure(err)
	}
	result := PrimerResult{Epoch: selected, Question: strings.TrimSpace(request.Question)}

	view, template, err := p.openReportSet(ctx, selected)
	if err != nil {
		return result, err
	}
	result.CommunitySetID = view.CommunitySetID
	dimension, err := p.preflightReports(ctx, selected)
	if err != nil {
		return result, err
	}
	hypothetical, usage, err := p.generateHypothetical(
		ctx, selected.CorporaID, request.Question, template, budget,
	)
	if err != nil {
		result.Usage = usage
		return result, err
	}
	result.Usage = usage
	result.HypotheticalAnswer = hypothetical

	vector, err := p.embedder.Embed(ctx, hypothetical)
	if err != nil {
		return result, normalizeFailure(err)
	}
	if err = validateVector(vector); err != nil {
		return result, querybase.NewInvalidModelResponseFailure(err)
	}
	if len(vector) != dimension {
		return result, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
			"DRIFT hypothetical vector dimension %d does not match Report vector dimension %d",
			len(vector),
			dimension,
		))
	}
	reports, err := p.retrieveReports(ctx, selected, view, vector)
	if err != nil {
		return result, err
	}
	result.Reports = reports
	folds, foldUsage, err := p.analyzeFolds(
		ctx, selected.CorporaID,
		request.Question,
		reports,
		budget,
	)
	if err != nil {
		result.Usage = addUsage(result.Usage, foldUsage)
		return result, err
	}
	result.Usage = addUsage(result.Usage, foldUsage)
	result.Folds = folds
	for _, fold := range result.Folds {
		result.IntermediateAnswer = joinAnswer(
			result.IntermediateAnswer,
			fold.Response.IntermediateAnswer,
		)
		result.Score += float64(fold.Response.Score)
		result.FollowUpQueries = append(
			result.FollowUpQueries,
			fold.Response.FollowUpQueries...,
		)
	}
	result.Score /= float64(len(result.Folds))
	return result, nil
}

func (p *Primer) generateHypothetical(
	ctx context.Context,
	corporaID string,
	question string,
	template string,
	budget *requestBudget,
) (string, Usage, error) {
	prompt, tokens, _, err := p.renderWithinBudget(
		ctx, corporaID,
		p.hypotheticalPrompt, map[string]string{
			"query":    question,
			"template": template,
		}, "template")
	usage := Usage{PromptTokens: tokens}
	if err != nil {
		return "", usage, err
	}
	if budget != nil {
		if !budget.reserve(tokens, p.config.HyDEMaxCompletionTokens) {
			return "", usage, querybase.NewInternalFailure(
				errors.New("validated DRIFT request budget cannot contain HyDE"),
			)
		}
	}
	response, modelErr := p.model.GenerateHypothetical(ctx,
		ModelRequest{
			Prompt:              prompt,
			MaxCompletionTokens: p.config.HyDEMaxCompletionTokens,
		},
	)
	usage.Calls = 1
	if modelErr != nil {
		if budget != nil {
			budget.finish(p.config.HyDEMaxCompletionTokens, 0)
		}
		return "", usage, normalizeFailure(modelErr)
	}
	usage.OutputTokens, err = p.countOutput(
		ctx, corporaID, response, p.config.HyDEMaxCompletionTokens, "HyDE",
	)
	if budget != nil {
		budget.finish(p.config.HyDEMaxCompletionTokens, usage.OutputTokens)
	}
	if err != nil {
		return "", usage, err
	}
	if strings.TrimSpace(response) == "" {
		return question, usage, nil
	}
	return response, usage, nil
}

func (p *Primer) renderWithinBudget(
	ctx context.Context,
	corporaID string,
	template string,
	fields map[string]string,
	trimmableField string,
) (string, int, string, error) {
	content, exists := fields[trimmableField]
	if !exists {
		return "", 0, "", querybase.NewInternalFailure(
			fmt.Errorf("DRIFT Primer prompt field %q is missing", trimmableField),
		)
	}
	values := make(map[string]string, len(fields))
	for name, value := range fields {
		values[name] = value
	}
	prefix, fit, err := fittingPrefix(content, func(candidate string) (bool, error) {
		values[trimmableField] = candidate
		_, count, err := p.renderAndCount(ctx, corporaID, template, values)
		return count <= p.config.MaxPromptTokens, err
	})
	if err != nil {
		return "", 0, "", err
	}
	if !fit {
		return "", 0, "", querybase.NewInvalidInputFailure(
			"the DRIFT question cannot fit the configured Primer prompt token limit",
			fmt.Errorf("DRIFT Primer fixed prompt exceeds %d tokens", p.config.MaxPromptTokens),
		)
	}
	values[trimmableField] = prefix
	rendered, count, err := p.renderAndCount(ctx, corporaID, template, values)
	return rendered, count, prefix, err
}

func (p *Primer) renderAndCount(
	ctx context.Context,
	corporaID string,
	template string,
	fields map[string]string,
) (string, int, error) {
	rendered, err := queryprompt.Render(template, fields)
	if err != nil {
		return "", 0, querybase.NewInvalidInputFailure(
			"the DRIFT Primer prompt is invalid", err,
		)
	}
	count, err := p.tokens.Count(ctx, corporaID, rendered)
	if err != nil {
		return "", 0, normalizeFailure(err)
	}
	if count < 0 {
		return "", 0, querybase.NewInternalFailure(
			errors.New("DRIFT TokenCounter returned a negative Primer prompt count"),
		)
	}
	return rendered, count, nil
}

func (p *Primer) countOutput(
	ctx context.Context,
	corporaID string,
	output string,
	limit int,
	stage string,
) (int, error) {
	count, err := p.tokens.Count(ctx, corporaID, output)
	if err != nil {
		return 0, normalizeFailure(err)
	}
	if count < 0 {
		return 0, querybase.NewInternalFailure(
			errors.New("DRIFT TokenCounter returned a negative Primer output count"),
		)
	}
	if count > limit {
		return count, querybase.NewInvalidModelResponseFailure(fmt.Errorf(
			"DRIFT %s output contains %d tokens, limit is %d",
			stage,
			count,
			limit,
		))
	}
	return count, nil
}

func validatePrompt(template string, fields map[string]string, markers []string) error {
	if strings.TrimSpace(template) == "" {
		return errors.New("prompt is required")
	}
	rendered, err := queryprompt.Render(template, fields)
	if err != nil {
		return err
	}
	for _, marker := range markers {
		if !strings.Contains(rendered, marker) {
			return errors.New("prompt omits a required field")
		}
	}
	return nil
}

func validateVector(vector []float64) error {
	for _, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("DRIFT query vector contains a non-finite value")
		}
	}
	return nil
}

func joinAnswer(current string, next string) string {
	if current == "" {
		return next
	}
	return current + "\n\n" + next
}

func addUsage(left Usage, right Usage) Usage {
	return Usage{
		Calls:        left.Calls + right.Calls,
		PromptTokens: left.PromptTokens + right.PromptTokens,
		OutputTokens: left.OutputTokens + right.OutputTokens,
	}
}
