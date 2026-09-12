package drift

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	querycitation "github.com/memoria-space/meking/query/internal/citation"
)

type intermediateCandidate struct {
	question        string
	response        string
	score           float64
	original        []querybase.CitationReference
	records         []querybase.CitationRecord
	additional      bool
	primerSupported bool
}

// boundCandidate is one answer candidate with provisional request-global
// Citation references. Its ledger becomes current only after the answer fits.
type boundCandidate struct {
	candidate    intermediateCandidate
	ledger       *citationLedger
	replacements map[querybase.CitationReference]querybase.CitationReference
	assigned     []querybase.CitationReference
}

func (r *Reducer) buildContext(
	ctx context.Context,
	traversal TraversalResult,
	responseType string,
	maxPromptTokens int,
) (ReduceContext, error) {
	candidates, err := reduceCandidates(traversal)
	if err != nil {
		return ReduceContext{}, err
	}
	ledger := newCitationLedger()
	blocks := make([]string, 0, len(candidates))
	result := ReduceContext{}
	for _, candidate := range candidates {
		bound := bindCandidate(candidate, ledger)
		answer, block := bound.render(len(result.Answers), candidate.response)
		candidateText := strings.Join(append(append([]string(nil), blocks...), block), "\n\n")
		count, fits, err := r.measureContext(
			ctx, traversal, responseType, maxPromptTokens, candidateText,
		)
		if err != nil {
			return result, err
		}
		if !fits {
			result.Truncated = true
			if len(result.Answers) > 0 {
				break
			}
			answer, block, count, err = r.truncateFirstAnswer(
				ctx, traversal, responseType, maxPromptTokens, bound,
			)
			if err != nil {
				return result, err
			}
			candidateText = block
		}
		ledger = bound.ledger
		blocks = append(blocks, block)
		result.Answers = append(result.Answers, answer)
		result.Text = candidateText
		result.TokenCount = count
		if result.Truncated {
			break
		}
	}
	result.CitationRecords = copyCitationRecords(ledger.records)
	return result, nil
}

func bindCandidate(candidate intermediateCandidate, ledger *citationLedger) boundCandidate {
	bound := boundCandidate{
		candidate:    candidate,
		ledger:       ledger.clone(),
		replacements: make(map[querybase.CitationReference]querybase.CitationReference),
		assigned:     make([]querybase.CitationReference, len(candidate.records)),
	}
	for index, record := range candidate.records {
		bound.assigned[index] = bound.ledger.bind(record)
		if index < len(candidate.original) {
			bound.replacements[candidate.original[index]] = bound.assigned[index]
		}
	}
	return bound
}

func (c boundCandidate) render(index int, raw string) (IntermediateAnswer, string) {
	response := querycitation.RewriteReferences(raw, c.replacements)
	if c.candidate.primerSupported {
		response = appendReportReferences(response, c.assigned, c.candidate.additional)
	}
	answer := IntermediateAnswer{
		Question: c.candidate.question,
		Response: response,
		Score:    c.candidate.score,
	}
	return answer, formatIntermediateAnswer(index, answer)
}

func (r *Reducer) measureContext(
	ctx context.Context,
	traversal TraversalResult,
	responseType string,
	maxPromptTokens int,
	text string,
) (int, bool, error) {
	count, err := r.tokens.Count(ctx, traversal.Primer.Epoch.CorporaID, text)
	if err != nil {
		return 0, false, normalizeFailure(err)
	}
	if count < 0 {
		return 0, false, querybase.NewInternalFailure(
			errors.New("DRIFT TokenCounter returned a negative Reduce context count"),
		)
	}
	request, err := r.modelRequest(
		traversal.Primer.Question,
		responseType,
		ReduceContext{Text: text},
	)
	if err != nil {
		return 0, false, err
	}
	promptTokens, err := r.countPrompt(
		ctx,
		traversal.Primer.Epoch.CorporaID,
		request.SystemPrompt,
		request.UserPrompt,
	)
	if err != nil {
		return 0, false, err
	}
	return count, count <= r.config.MaxContextTokens && promptTokens <= maxPromptTokens, nil
}

func (r *Reducer) truncateFirstAnswer(
	ctx context.Context,
	traversal TraversalResult,
	responseType string,
	maxPromptTokens int,
	candidate boundCandidate,
) (IntermediateAnswer, string, int, error) {
	if !candidate.candidate.primerSupported {
		return IntermediateAnswer{}, "", 0, querybase.NewInternalFailure(
			errors.New("DRIFT Reduce has no Primer answer to truncate"),
		)
	}
	prefix, fit, err := fittingPrefix(
		candidate.candidate.response,
		func(value string) (bool, error) {
			_, block := candidate.render(0, value)
			_, fits, err := r.measureContext(ctx, traversal, responseType, maxPromptTokens, block)
			return fits, err
		},
	)
	if err != nil {
		return IntermediateAnswer{}, "", 0, err
	}
	if !fit || strings.TrimSpace(prefix) == "" {
		return IntermediateAnswer{}, "", 0, querybase.NewInvalidInputFailure(
			"the DRIFT question and minimum Report evidence cannot fit the configured Reduce token limits",
			fmt.Errorf("DRIFT Reduce fixed prompt exceeds its token limits"),
		)
	}
	answer, block := candidate.render(0, prefix)
	count, _, err := r.measureContext(ctx, traversal, responseType, maxPromptTokens, block)
	return answer, block, count, err
}

func reduceCandidates(traversal TraversalResult) ([]intermediateCandidate, error) {
	reports := make(map[string]SelectedReport, len(traversal.Primer.Reports))
	for _, report := range traversal.Primer.Reports {
		reports[report.Report.ID] = report
	}
	result := make([]intermediateCandidate, 0, len(traversal.Primer.Folds)+len(traversal.Branches))
	for _, fold := range traversal.Primer.Folds {
		limit := min(len(fold.ReportIDs), querycitation.MaximumRecordIDs)
		candidate := intermediateCandidate{
			question: traversal.Primer.Question, response: fold.Response.IntermediateAnswer,
			score: float64(fold.Response.Score), additional: len(fold.ReportIDs) > limit,
			primerSupported: true,
		}
		for _, id := range fold.ReportIDs[:limit] {
			report, exists := reports[id]
			if !exists {
				return nil, querybase.NewInternalFailure(
					fmt.Errorf("DRIFT Primer fold references unknown Report %q", id),
				)
			}
			candidate.records = append(candidate.records, querybase.CitationRecord{
				Reference:   querybase.CitationReference{Dataset: querybase.CitationReports},
				CorporaID:   traversal.Primer.Epoch.CorporaID,
				TextUnitIDs: append([]string(nil), report.Report.Sources.TextUnitIDs...),
			})
		}
		if len(candidate.records) == 0 {
			return nil, querybase.NewInternalFailure(
				errors.New("DRIFT Primer fold has no Report evidence"),
			)
		}
		result = append(result, candidate)
	}
	for _, branch := range traversal.Branches {
		if branch.Status != BranchSucceeded {
			continue
		}
		candidate, err := branchCandidate(branch)
		if err != nil {
			return nil, err
		}
		result = append(result, candidate)
	}
	return result, nil
}

func branchCandidate(branch Branch) (intermediateCandidate, error) {
	records := branch.Evidence.Context.CitationRecords()
	byReference := make(map[querybase.CitationReference]querybase.CitationRecord, len(records))
	for _, record := range records {
		byReference[record.Reference] = record
	}
	candidate := intermediateCandidate{
		question: branch.Question, response: branch.Answer, score: float64(branch.Score),
	}
	seen := make(map[querybase.CitationReference]struct{})
	for _, item := range branch.CitationAudit.Items {
		if item.Status != querybase.CitationValid {
			continue
		}
		if _, duplicate := seen[item.Reference]; duplicate {
			continue
		}
		record, exists := byReference[item.Reference]
		if !exists {
			return intermediateCandidate{}, querybase.NewInternalFailure(fmt.Errorf(
				"DRIFT branch audit accepted unknown %s record %d",
				item.Reference.Dataset,
				item.Reference.RecordID,
			))
		}
		seen[item.Reference] = struct{}{}
		candidate.original = append(candidate.original, item.Reference)
		candidate.records = append(candidate.records, record)
	}
	return candidate, nil
}

func appendReportReferences(
	response string,
	references []querybase.CitationReference,
	additional bool,
) string {
	ids := make([]string, 0, len(references)+1)
	for _, reference := range references {
		ids = append(ids, strconv.Itoa(reference.RecordID))
	}
	if additional {
		ids = append(ids, "+more")
	}
	response = strings.TrimRight(response, " \t\r\n")
	return response + "\n\n[Data: Reports (" + strings.Join(ids, ", ") + ")]"
}

func formatIntermediateAnswer(index int, answer IntermediateAnswer) string {
	return fmt.Sprintf(
		"----Intermediate Answer %d----\nQuestion: %q\nImportance Score: %s\n%s",
		index+1,
		answer.Question,
		strconv.FormatFloat(answer.Score, 'f', -1, 64),
		answer.Response,
	)
}
