package drift

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
	querylocal "github.com/memoria-space/meking/query/local"
	queryreport "github.com/memoria-space/meking/query/report"
	"golang.org/x/sync/errgroup"
)

var errRequestDuration = errors.New("DRIFT request duration exhausted")

// Traversal executes the bounded Local branch graph after a successful Primer.
// It never resolves Current and does not perform final reduction or Citation
// renumbering.
type Traversal struct {
	model  BranchModel
	tokens querybase.TokenCounter
	prompt string
	config TraversalConfig
}

// NewTraversal creates the provider-independent DRIFT Local branch stage.
func NewTraversal(
	model BranchModel,
	tokens querybase.TokenCounter,
	prompt string,
	config TraversalConfig,
) (*Traversal, error) {
	if model == nil {
		return nil, errors.New("create DRIFT Traversal: Branch model is required")
	}
	if tokens == nil {
		return nil, errors.New("create DRIFT Traversal: TokenCounter is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := validatePrompt(
		prompt,
		map[string]string{
			"context_data": "\x00context\x00", "response_type": "\x00response\x00",
			"global_query": "\x00global\x00", "followups": "\x00followups\x00",
		},
		[]string{"\x00context\x00", "\x00response\x00", "\x00global\x00", "\x00followups\x00"},
	); err != nil {
		return nil, fmt.Errorf("validate DRIFT branch prompt: %w", err)
	}
	return &Traversal{model: model, tokens: tokens, prompt: prompt, config: config}, nil
}

// Explore uses stable BFS discovery, reuses duplicate question nodes, retains
// every generated edge, and isolates non-cancellation branch failures.
func (t *Traversal) Explore(
	parent context.Context,
	request TraversalRequest,
	session LocalSession,
) (result TraversalResult, resultErr error) {
	if t == nil {
		return result, querybase.NewInternalFailure(errors.New("DRIFT Traversal is not configured"))
	}
	workflow, cancel := context.WithTimeoutCause(
		parent,
		t.config.Limits.Duration,
		errRequestDuration,
	)
	defer cancel()
	return t.explore(parent, workflow, request, session, nil)
}

func (t *Traversal) explore(
	parent context.Context,
	workflow context.Context,
	request TraversalRequest,
	session LocalSession,
	budget *requestBudget,
) (result TraversalResult, resultErr error) {
	if t == nil || t.model == nil || t.tokens == nil {
		return result, querybase.NewInternalFailure(errors.New("DRIFT Traversal is not configured"))
	}
	if session == nil {
		return result, querybase.NewInternalFailure(errors.New("DRIFT Local Session is required"))
	}
	if strings.TrimSpace(request.Primer.Question) == "" {
		return result, querybase.NewInvalidInputFailure("DRIFT Primer question is required", nil)
	}
	if !session.Epoch().Equal(request.Primer.Epoch) {
		return result, querybase.NewInternalFailure(
			errors.New("DRIFT Local Session does not match the Primer Epoch"),
		)
	}
	responseType := request.ResponseType
	if responseType == "" {
		responseType = t.config.ResponseType
	} else if strings.TrimSpace(responseType) == "" || responseType != strings.TrimSpace(responseType) {
		return result, querybase.NewInvalidInputFailure(
			"DRIFT response type must not have surrounding whitespace",
			nil,
		)
	}
	if err := parent.Err(); err != nil {
		return result, normalizeFailure(err)
	}

	result = TraversalResult{
		Primer: clonePrimer(request.Primer),
	}
	if budget == nil {
		budget = newRequestBudget(t.config.Limits, request.Primer.Usage)
	}
	defer func() {
		result.Usage = budget.usage
	}()
	graph := newBranchGraph(&result, t.config.MaxDepth)
	defer graph.discardPending()

	processed := 0
	for {
		if stop := stopTraversal(graph, parent, workflow); stop != nil {
			return result, stop
		}
		remaining := t.config.MaxBranches - processed
		if remaining <= 0 {
			break
		}
		indices := graph.pending(min(t.config.BatchSize, remaining))
		if len(indices) == 0 {
			break
		}
		capacity := budget.capacity(t.config.MaxCompletionTokens)
		if capacity == 0 {
			break
		}
		if len(indices) > capacity {
			indices = indices[:capacity]
		}
		processed += len(indices)
		questions := make([]string, len(indices))
		for position, index := range indices {
			questions[position] = result.Branches[index].Question
		}
		evidence := t.gatherEvidence(workflow, questions, session)
		if stop := stopTraversal(graph, parent, workflow); stop != nil {
			return result, stop
		}
		prepared := make([]preparedBranch, 0, len(indices))
		budgetStopped := false
		for position, index := range indices {
			branch := &result.Branches[index]
			if evidence[position].err != nil {
				branch.Status = BranchFailed
				branch.Failure = branchFailure(evidence[position].err)
				continue
			}
			branch.Evidence = evidence[position].value
			systemPrompt, err := queryprompt.Render(t.prompt, map[string]string{
				"context_data": branch.Evidence.Context.Text, "response_type": responseType,
				"global_query": request.Primer.Question,
				"followups":    strconv.Itoa(t.config.FollowUpLimit),
			})
			if err != nil {
				return result, querybase.NewInternalFailure(fmt.Errorf("render DRIFT branch prompt: %w", err))
			}
			promptTokens, err := t.countPrompt(
				workflow,
				request.Primer.Epoch.CorporaID,
				systemPrompt,
				branch.Question,
			)
			if err != nil {
				if stop := stopTraversal(graph, parent, workflow); stop != nil {
					return result, stop
				}
				return result, normalizeFailure(err)
			}
			if !budget.reserve(promptTokens, t.config.MaxCompletionTokens) {
				budgetStopped = true
				break
			}
			branch.Usage = Usage{Calls: 1, PromptTokens: promptTokens}
			prepared = append(prepared,
				preparedBranch{
					index: index,
					request: BranchModelRequest{
						SystemPrompt: systemPrompt, UserPrompt: branch.Question,
						MaxCompletionTokens: t.config.MaxCompletionTokens,
					},
				},
			)
		}
		outcomes := t.run(workflow, prepared)
		if stop := stopTraversal(graph, parent, workflow); stop != nil {
			return result, stop
		}
		for position, current := range prepared {
			branch := &result.Branches[current.index]
			outcome := outcomes[position]
			if outcome.err != nil {
				budget.finish(t.config.MaxCompletionTokens, 0)
				branch.Status = BranchFailed
				branch.Failure = branchFailure(outcome.err)
				continue
			}
			outputTokens, err := t.tokens.Count(
				workflow,
				request.Primer.Epoch.CorporaID,
				outcome.response,
			)
			if err != nil {
				if stop := stopTraversal(graph, parent, workflow); stop != nil {
					return result, stop
				}
				return result, normalizeFailure(err)
			}
			if outputTokens < 0 {
				return result, querybase.NewInternalFailure(errors.New("DRIFT TokenCounter returned a negative output count"))
			}
			branch.Usage.OutputTokens = outputTokens
			withinOutputLimit := budget.finish(t.config.MaxCompletionTokens, outputTokens)
			if !withinOutputLimit {
				branch.Status = BranchFailed
				branch.Failure = querybase.NewInvalidModelResponseFailure(fmt.Errorf(
					"DRIFT branch output contains %d tokens, limit is %d",
					outputTokens,
					t.config.MaxCompletionTokens,
				))
				continue
			}
			parsed, err := parseBranchResponse(outcome.response, t.config.FollowUpLimit)
			if err != nil {
				corrector, supported := t.model.(BranchCorrector)
				if !supported {
					branch.Status = BranchFailed
					branch.Failure = querybase.NewInvalidModelResponseFailure(err)
					continue
				}
				correction := BranchCorrection{
					Request: current.request,
					Result:  outcome.response,
					Reason:  err.Error(),
				}
				feedbackTokens, countErr := t.tokens.Count(
					workflow,
					request.Primer.Epoch.CorporaID,
					correction.Feedback(),
				)
				if countErr != nil || feedbackTokens < 0 {
					if countErr == nil {
						countErr = errors.New("DRIFT TokenCounter returned a negative correction prompt count")
					}
					return result, normalizeFailure(countErr)
				}
				correctionPromptTokens := branch.Usage.PromptTokens + outputTokens + feedbackTokens
				if !budget.reserve(correctionPromptTokens, t.config.MaxCompletionTokens) {
					branch.Status = BranchFailed
					branch.Failure = querybase.NewInvalidModelResponseFailure(err)
					continue
				}
				branch.Usage.Calls++
				branch.Usage.PromptTokens += correctionPromptTokens
				corrected, correctionErr := corrector.CorrectBranch(workflow, correction)
				if correctionErr != nil {
					budget.finish(t.config.MaxCompletionTokens, 0)
					branch.Status = BranchFailed
					branch.Failure = branchFailure(correctionErr)
					continue
				}
				correctedTokens, correctedCountErr := t.tokens.Count(
					workflow,
					request.Primer.Epoch.CorporaID,
					corrected,
				)
				if correctedCountErr != nil || correctedTokens < 0 {
					if correctedCountErr == nil {
						correctedCountErr = errors.New("DRIFT TokenCounter returned a negative corrected output count")
					}
					budget.finish(t.config.MaxCompletionTokens, 0)
					return result, normalizeFailure(correctedCountErr)
				}
				branch.Usage.OutputTokens += correctedTokens
				if !budget.finish(t.config.MaxCompletionTokens, correctedTokens) ||
					correctedTokens > t.config.MaxCompletionTokens {
					branch.Status = BranchFailed
					branch.Failure = querybase.NewInvalidModelResponseFailure(fmt.Errorf(
						"DRIFT corrected branch output contains %d tokens, limit is %d",
						correctedTokens,
						t.config.MaxCompletionTokens,
					))
					continue
				}
				parsed, err = parseBranchResponse(corrected, t.config.FollowUpLimit)
				if err != nil {
					branch.Status = BranchFailed
					branch.Failure = querybase.NewInvalidModelResponseFailure(err)
					continue
				}
			}
			branch.Answer = parsed.answer
			branch.Score = parsed.score
			branch.FollowUpQueries = append([]string(nil), parsed.followUps...)
			branch.Status = BranchSucceeded
			graph.add(branch.Question, branch.Depth, branch.FollowUpQueries, t.config.MaxDepth)
		}
		if budgetStopped {
			break
		}
	}
	return result, nil
}

type preparedBranch struct {
	index   int
	request BranchModelRequest
}

type evidenceOutcome struct {
	value querylocal.Evidence
	err   error
}

func (t *Traversal) gatherEvidence(
	ctx context.Context,
	questions []string,
	session LocalSession,
) []evidenceOutcome {
	result := make([]evidenceOutcome, len(questions))
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(t.config.MaxConcurrency)
	for index := range questions {
		index := index
		group.Go(func() error {
			result[index].value, result[index].err = session.Evidence(
				groupContext,
				querylocal.SearchRequest{Question: questions[index]},
			)
			return nil
		})
	}
	_ = group.Wait()
	return result
}

type branchOutcome struct {
	response string
	err      error
}

func (t *Traversal) run(ctx context.Context, prepared []preparedBranch) []branchOutcome {
	result := make([]branchOutcome, len(prepared))
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(t.config.MaxConcurrency)
	for index := range prepared {
		idx := index
		group.Go(func() error {
			result[idx].response, result[idx].err = t.model.GenerateBranch(
				groupContext,
				prepared[idx].request,
			)
			return nil
		})
	}
	_ = group.Wait()
	return result
}

func (t *Traversal) countPrompt(
	ctx context.Context,
	corporaID string,
	system string,
	user string,
) (int, error) {
	systemTokens, err := t.tokens.Count(ctx, corporaID, system)
	if err != nil {
		return 0, err
	}
	if systemTokens < 0 {
		return 0, errors.New("DRIFT TokenCounter returned a negative system prompt count")
	}
	userTokens, err := t.tokens.Count(ctx, corporaID, user)
	if err != nil {
		return 0, err
	}
	if userTokens < 0 {
		return 0, errors.New("DRIFT TokenCounter returned a negative user prompt count")
	}
	return systemTokens + userTokens, nil
}

func stopTraversal(
	graph *branchGraph,
	parent context.Context,
	workflow context.Context,
) *querybase.Failure {
	if err := parent.Err(); err != nil {
		failure := querybase.NewCancelledFailure(err)
		graph.failPending(failure)
		return failure
	}
	if errors.Is(context.Cause(workflow), errRequestDuration) {
		return querybase.NewBudgetExceededFailure(errRequestDuration)
	}
	return nil
}

func branchFailure(err error) *querybase.Failure {
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewInternalFailure(err)
}

func clonePrimer(value PrimerResult) PrimerResult {
	result := value
	result.Reports = make([]SelectedReport, len(value.Reports))
	for index, selected := range value.Reports {
		selected.Report.Findings = append([]queryreport.ReportFinding(nil), selected.Report.Findings...)
		selected.Report.Sources.Entities = append(
			[]querybase.KnowledgeReference(nil),
			selected.Report.Sources.Entities...,
		)
		selected.Report.Sources.Relations = append(
			[]querybase.KnowledgeReference(nil),
			selected.Report.Sources.Relations...,
		)
		selected.Report.Sources.Claims = append(
			[]querybase.ClaimReference(nil),
			selected.Report.Sources.Claims...,
		)
		selected.Report.Sources.TextUnitIDs = append(
			[]string(nil),
			selected.Report.Sources.TextUnitIDs...,
		)
		result.Reports[index] = selected
	}
	result.Folds = make([]PrimerFold, len(value.Folds))
	for index, fold := range value.Folds {
		fold.ReportIDs = append([]string(nil), fold.ReportIDs...)
		fold.Response.FollowUpQueries = append(
			[]string(nil),
			fold.Response.FollowUpQueries...,
		)
		result.Folds[index] = fold
	}
	result.FollowUpQueries = append([]string(nil), value.FollowUpQueries...)
	return result
}
