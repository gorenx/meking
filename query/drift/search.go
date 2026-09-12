package drift

import (
	"context"
	"errors"
	"fmt"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	querycitation "github.com/memoria-space/meking/query/internal/citation"
	querysource "github.com/memoria-space/meking/query/source"
)

type SearchRequest struct {
	Question     string
	ResponseType string
}

// Result is one complete DRIFT answer. Traversal retains the immutable Primer
// and Local branch evidence; ReduceContext is the only Citation scope used by
// the final answer.
type Result struct {
	Response      string
	Traversal     TraversalResult
	ReduceContext ReduceContext
	Usage         Usage
	CitationAudit querybase.CitationAudit
}

// Searcher owns one request-wide DRIFT execution: a Report Primer, Local
// traversal, branch Citation audit, stable evidence merge, and final Reduce.
type Searcher struct {
	primer    *Primer
	locals    LocalSessionOpener
	traversal *Traversal
	reducer   *Reducer
	sources   querysource.Reader
	limits    RequestLimits
}

func NewSearcher(
	primer *Primer,
	locals LocalSessionOpener,
	traversal *Traversal,
	reducer *Reducer,
	sources querysource.Reader,
) (*Searcher, error) {
	switch {
	case primer == nil:
		return nil, errors.New("create DRIFT Searcher: Primer is required")
	case locals == nil:
		return nil, errors.New("create DRIFT Searcher: LocalSessionOpener is required")
	case traversal == nil:
		return nil, errors.New("create DRIFT Searcher: Traversal is required")
	case reducer == nil:
		return nil, errors.New("create DRIFT Searcher: Reducer is required")
	case sources == nil:
		return nil, errors.New("create DRIFT Searcher: Source Reader is required")
	}
	configuration := Configuration{
		Primer: primer.config, Traversal: traversal.config, Reduce: reducer.config,
	}
	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("create DRIFT Searcher: %w", err)
	}
	limits := traversal.config.Limits
	if limits != reducer.limits {
		return nil, errors.New("create DRIFT Searcher: Traversal and Reducer request limits differ")
	}
	return &Searcher{
		primer: primer, locals: locals,
		traversal: traversal, reducer: reducer,
		sources: sources, limits: limits,
	}, nil
}

func (s *Searcher) Search(ctx context.Context, request SearchRequest) (Result, error) {
	workflow, cancel, budget, err := s.start(ctx, request)
	if err != nil {
		return Result{}, err
	}
	defer cancel()

	traversalResult, err := s.traverse(ctx, workflow, request, budget)
	if err != nil {
		return Result{Traversal: traversalResult, Usage: traversalResult.Usage},
			requestFailure(ctx, workflow, err)
	}

	traversalResult, err = s.auditBranches(workflow, traversalResult)
	if err != nil {
		return Result{Traversal: traversalResult, Usage: traversalResult.Usage},
			requestFailure(ctx, workflow, err)
	}

	reduction, err := s.reducer.reduce(workflow, ReduceRequest{
		Traversal: traversalResult, ResponseType: request.ResponseType,
	}, budget)
	if err != nil {
		return assembleResult(traversalResult, reduction), requestFailure(ctx, workflow, err)
	}

	result, err := s.auditFinal(workflow, assembleResult(traversalResult, reduction))
	if err != nil {
		return result, requestFailure(ctx, workflow, err)
	}
	return result, nil
}

// Stream emits only the final Reduce. Primer and Local branch model output
// remains internal evidence; a failure after emitted text is marked partial.
func (s *Searcher) Stream(
	ctx context.Context,
	request SearchRequest,
	emit querybase.TextDeltaHandler,
) (Result, error) {
	if emit == nil {
		return Result{}, querybase.NewInvalidInputFailure(
			"DRIFT stream handler is required", nil,
		)
	}
	workflow, cancel, budget, err := s.start(ctx, request)
	if err != nil {
		return Result{}, err
	}
	defer cancel()

	traversal, err := s.traverse(ctx, workflow, request, budget)
	if err != nil {
		return Result{Traversal: traversal, Usage: traversal.Usage},
			requestFailure(ctx, workflow, err)
	}
	traversal, err = s.auditBranches(workflow, traversal)
	if err != nil {
		return Result{Traversal: traversal, Usage: traversal.Usage},
			requestFailure(ctx, workflow, err)
	}
	reduction, err := s.reducer.stream(
		workflow,
		ReduceRequest{
			Traversal:    traversal,
			ResponseType: request.ResponseType,
		},
		emit, budget,
	)
	if err != nil {
		result := assembleResult(traversal, reduction)
		return result, partialFailure(result.Response, requestFailure(ctx, workflow, err))
	}
	result, err := s.auditFinal(workflow, assembleResult(traversal, reduction))
	if err != nil {
		return result, partialFailure(result.Response, requestFailure(ctx, workflow, err))
	}
	return result, nil
}

func (s *Searcher) start(
	parent context.Context,
	request SearchRequest,
) (context.Context, context.CancelFunc, *requestBudget, error) {
	if s == nil || s.primer == nil || s.locals == nil || s.traversal == nil ||
		s.reducer == nil || s.sources == nil {
		return nil, nil, nil, querybase.NewInternalFailure(
			errors.New("DRIFT Searcher is not configured"),
		)
	}
	if err := validateSearchRequest(request); err != nil {
		return nil, nil, nil, querybase.NewInvalidInputFailure(err.Error(), err)
	}
	if err := parent.Err(); err != nil {
		return nil, nil, nil, querybase.NewCancelledFailure(err)
	}
	workflow, cancel := context.WithTimeoutCause(parent, s.limits.Duration, errRequestDuration)
	budget := newRequestBudget(s.limits, Usage{})
	if !budget.hold(1, s.reducer.config.MaxCompletionTokens) {
		cancel()
		return nil, nil, nil, querybase.NewInternalFailure(
			errors.New("DRIFT final Reduce cannot reserve request budget"),
		)
	}
	return workflow, cancel, budget, nil
}

func (s *Searcher) traverse(
	parent context.Context,
	workflow context.Context,
	request SearchRequest,
	budget *requestBudget,
) (result TraversalResult, resultErr error) {
	primer, err := s.primer.search(workflow, PrimerRequest{Question: request.Question}, budget)
	if err != nil {
		return TraversalResult{Primer: primer, Usage: primer.Usage}, err
	}
	result = TraversalResult{Primer: primer, Usage: primer.Usage}
	session, err := s.locals.Open(workflow, primer.Epoch)
	if err != nil {
		return result, normalizeFailure(err)
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				querybase.NewInternalFailure(fmt.Errorf("close DRIFT Local Session: %w", closeErr)),
			)
		}
	}()
	traversed, err := s.traversal.explore(
		parent, workflow,
		TraversalRequest{
			Primer:       primer,
			ResponseType: request.ResponseType,
		},
		session,
		budget,
	)
	if err != nil {
		return traversed, err
	}
	return traversed, nil
}

func (s *Searcher) auditBranches(
	ctx context.Context,
	result TraversalResult,
) (TraversalResult, error) {
	for index := range result.Branches {
		branch := &result.Branches[index]
		if branch.Status != BranchSucceeded {
			continue
		}
		audit, err := querycitation.Audit(
			ctx,
			branch.Answer,
			branch.Evidence.Context.CitationRecords(),
			s.sources,
		)
		if err != nil {
			return result, normalizeFailure(err)
		}
		branch.CitationAudit = audit
	}
	return result, nil
}

func (s *Searcher) auditFinal(ctx context.Context, result Result) (Result, error) {
	audit, err := querycitation.Audit(
		ctx, result.Response, result.ReduceContext.CitationRecords, s.sources,
	)
	if err != nil {
		return result, normalizeFailure(err)
	}
	result.CitationAudit = audit
	return result, nil
}

func assembleResult(traversal TraversalResult, reduction Reduction) Result {
	return Result{
		Response:      reduction.Response,
		Traversal:     traversal,
		ReduceContext: reduction.Context,
		Usage:         reduction.Usage,
	}
}

func validateSearchRequest(request SearchRequest) error {
	if strings.TrimSpace(request.Question) == "" {
		return errors.New("DRIFT question is required")
	}
	if request.ResponseType != "" && (strings.TrimSpace(request.ResponseType) == "" ||
		request.ResponseType != strings.TrimSpace(request.ResponseType)) {
		return errors.New("DRIFT response type must not have surrounding whitespace")
	}
	return nil
}

func requestFailure(parent context.Context, workflow context.Context, err error) error {
	if parent.Err() != nil {
		return querybase.NewCancelledFailure(parent.Err())
	}
	if errors.Is(context.Cause(workflow), errRequestDuration) {
		return querybase.NewBudgetExceededFailure(errRequestDuration)
	}
	return normalizeFailure(err)
}

func partialFailure(response string, err error) error {
	if response == "" {
		return err
	}
	return querybase.MarkPartialOutput(err)
}
