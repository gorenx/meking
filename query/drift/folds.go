package drift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	"golang.org/x/sync/errgroup"
)

func (p *Primer) analyzeFolds(
	ctx context.Context,
	corporaID string,
	question string,
	reports []SelectedReport,
	budget *requestBudget,
) ([]PrimerFold, Usage, error) {
	groups := splitReports(reports, p.config.Folds)
	prepared := make([]preparedFold, len(groups))
	usage := Usage{}
	for index := range groups {
		fold, err := p.prepareFold(ctx, corporaID, question, groups[index])
		if err != nil {
			return nil, usage, err
		}
		prepared[index] = fold
	}
	if budget != nil {
		reserved := 0
		for _, fold := range prepared {
			if !budget.reserve(fold.promptTokens, p.config.PrimerMaxCompletionTokens) {
				break
			}
			reserved++
		}
		if reserved == 0 {
			return nil, Usage{}, querybase.NewInternalFailure(
				errors.New("validated DRIFT request budget cannot contain one Primer fold"),
			)
		}
		prepared = prepared[:reserved]
	}
	for _, fold := range prepared {
		usage.PromptTokens += fold.promptTokens
	}
	usage.Calls = len(prepared)
	outcomes := make([]foldOutcome, len(prepared))
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(p.config.MaxConcurrency)
	for index := range prepared {
		idx := index
		group.Go(func() error {
			outcomes[idx].response, outcomes[idx].err = p.model.GeneratePrimer(
				groupContext, prepared[idx].request,
			)
			return nil
		})
	}
	_ = group.Wait()
	result := make([]PrimerFold, len(prepared))
	var resultErr error

	for index, outcome := range outcomes {
		if outcome.err != nil {
			if budget != nil {
				budget.finish(p.config.PrimerMaxCompletionTokens, 0)
			}
			if resultErr == nil {
				resultErr = normalizeFailure(outcome.err)
			}
			continue
		}
		outputTokens, outputErr := p.countOutput(
			ctx, corporaID, outcome.response, p.config.PrimerMaxCompletionTokens, "Primer fold",
		)
		if budget != nil {
			budget.finish(p.config.PrimerMaxCompletionTokens, outputTokens)
		}
		usage.OutputTokens += outputTokens
		if outputErr != nil {
			if resultErr == nil {
				resultErr = outputErr
			}
			continue
		}
		response, err := parsePrimerResponse(outcome.response)
		if err != nil {
			corrector, supported := p.model.(PrimerCorrector)
			if !supported {
				if resultErr == nil {
					resultErr = querybase.NewInvalidModelResponseFailure(err)
				}
				continue
			}
			correction := PrimerCorrection{
				Request: prepared[index].request,
				Result:  outcome.response,
				Reason:  err.Error(),
			}
			feedbackTokens, countErr := p.tokens.Count(ctx, corporaID, correction.Feedback())
			if countErr != nil || feedbackTokens < 0 {
				if countErr == nil {
					countErr = errors.New("DRIFT TokenCounter returned a negative correction prompt count")
				}
				if resultErr == nil {
					resultErr = normalizeFailure(countErr)
				}
				continue
			}
			correctionPromptTokens := prepared[index].promptTokens + outputTokens + feedbackTokens
			if budget != nil && !budget.reserve(correctionPromptTokens, p.config.PrimerMaxCompletionTokens) {
				if resultErr == nil {
					resultErr = querybase.NewInvalidModelResponseFailure(err)
				}
				continue
			}
			usage.Calls++
			usage.PromptTokens += correctionPromptTokens
			corrected, correctionErr := corrector.CorrectPrimer(ctx, correction)
			if correctionErr != nil {
				if budget != nil {
					budget.finish(p.config.PrimerMaxCompletionTokens, 0)
				}
				if resultErr == nil {
					resultErr = normalizeFailure(correctionErr)
				}
				continue
			}
			correctedTokens, correctedCountErr := p.countOutput(
				ctx, corporaID, corrected, p.config.PrimerMaxCompletionTokens, "corrected Primer fold",
			)
			if budget != nil {
				budget.finish(p.config.PrimerMaxCompletionTokens, correctedTokens)
			}
			usage.OutputTokens += correctedTokens
			if correctedCountErr != nil {
				if resultErr == nil {
					resultErr = correctedCountErr
				}
				continue
			}
			response, err = parsePrimerResponse(corrected)
			if err != nil {
				if resultErr == nil {
					resultErr = querybase.NewInvalidModelResponseFailure(err)
				}
				continue
			}
			response.Usage = Usage{
				Calls:        2,
				PromptTokens: prepared[index].promptTokens + correctionPromptTokens,
				OutputTokens: outputTokens + correctedTokens,
			}
		} else {
			response.Usage = Usage{
				Calls: 1, PromptTokens: prepared[index].promptTokens, OutputTokens: outputTokens,
			}
		}
		result[index] = PrimerFold{
			Index: index, ReportIDs: prepared[index].reportIDs, Response: response,
		}
	}
	if resultErr != nil {
		return nil, usage, resultErr
	}
	return result, usage, nil
}

type preparedFold struct {
	reportIDs    []string
	promptTokens int
	request      ModelRequest
}

type foldOutcome struct {
	response string
	err      error
}

func (p *Primer) prepareFold(
	ctx context.Context,
	corporaID string,
	question string,
	reports []SelectedReport,
) (preparedFold, error) {
	if len(reports) == 0 {
		return preparedFold{}, querybase.NewNoEvidenceFailure(
			errors.New("DRIFT Primer fold has no Report"),
		)
	}
	reportIDs := make([]string, 0, len(reports))
	contents := make([]string, 0, len(reports))
	var prompt string
	var promptTokens int
	for _, selected := range reports {
		candidateContents := append(append([]string(nil), contents...), selected.Report.FullContent)
		fields := map[string]string{
			"query": question, "community_reports": strings.Join(candidateContents, "\n\n"),
		}
		candidate, count, err := p.renderAndCount(ctx, corporaID, p.primerPrompt, fields)
		if err != nil {
			return preparedFold{}, err
		}
		if count > p.config.MaxPromptTokens {
			if len(reportIDs) > 0 {
				break
			}
			var retained string
			candidate, count, retained, err = p.renderWithinBudget(
				ctx, corporaID, p.primerPrompt, fields, "community_reports",
			)
			if err != nil {
				return preparedFold{}, err
			}
			if strings.TrimSpace(retained) == "" {
				return preparedFold{}, querybase.NewInvalidInputFailure(
					"the DRIFT question leaves no Primer prompt capacity for Report evidence",
					nil,
				)
			}
			prompt, promptTokens = candidate, count
			reportIDs = append(reportIDs, selected.Report.ID)
			break
		}
		prompt, promptTokens = candidate, count
		contents = candidateContents
		reportIDs = append(reportIDs, selected.Report.ID)
	}
	return preparedFold{
		reportIDs:    reportIDs,
		promptTokens: promptTokens,
		request: ModelRequest{
			Prompt: prompt, MaxCompletionTokens: p.config.PrimerMaxCompletionTokens,
		},
	}, nil
}

func splitReports(reports []SelectedReport, folds int) [][]SelectedReport {
	if len(reports) == 0 {
		return nil
	}
	count := min(len(reports), folds)
	result := make([][]SelectedReport, count)
	base := len(reports) / count
	remainder := len(reports) % count
	start := 0
	for index := 0; index < count; index++ {
		size := base
		if index < remainder {
			size++
		}
		end := start + size
		result[index] = reports[start:end]
		start = end
	}
	return result
}

func parsePrimerResponse(response string) (PrimerResponse, error) {
	var wire struct {
		IntermediateAnswer string   `json:"intermediate_answer"`
		Score              *int     `json:"score"`
		FollowUpQueries    []string `json:"follow_up_queries"`
	}
	decoder := json.NewDecoder(strings.NewReader(response))
	if err := decoder.Decode(&wire); err != nil {
		return PrimerResponse{}, fmt.Errorf("decode DRIFT Primer response: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return PrimerResponse{}, errors.New("DRIFT Primer response contains trailing JSON")
		}
		return PrimerResponse{}, fmt.Errorf("decode DRIFT Primer response trailer: %w", err)
	}
	if strings.TrimSpace(wire.IntermediateAnswer) == "" {
		return PrimerResponse{}, errors.New("DRIFT Primer response has no intermediate answer")
	}
	if wire.Score == nil || *wire.Score < 0 || *wire.Score > 100 {
		return PrimerResponse{}, errors.New("DRIFT Primer response score must be between 0 and 100")
	}
	if len(wire.FollowUpQueries) == 0 {
		return PrimerResponse{}, errors.New("DRIFT Primer response has no follow-up queries")
	}
	followUps := make([]string, len(wire.FollowUpQueries))
	for index, question := range wire.FollowUpQueries {
		question = strings.TrimSpace(question)
		if question == "" {
			return PrimerResponse{}, fmt.Errorf(
				"DRIFT Primer follow-up query %d is empty", index,
			)
		}
		followUps[index] = question
	}
	return PrimerResponse{
		IntermediateAnswer: wire.IntermediateAnswer,
		Score:              *wire.Score,
		FollowUpQueries:    followUps,
	}, nil
}
