package local

import (
	"context"
	"errors"
	"sync"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

// Session fixes one Epoch, ReportSet view, and Entity-vector view for a bounded
// series of Local evidence builds.
type Session struct {
	// searcher owns the immutable retrieval, prompt, model, and Citation policy.
	searcher *Searcher
	// epoch is the immutable unified publication selected when the Session opened.
	epoch querybase.Epoch
	// reports is the isolated ReportSet view shared by every evidence build.
	reports queryreport.View
	// vectors is the Epoch-selected Entity-vector image used by every build.
	vectors EntityVectorReader
	// stateMu lets independent evidence builds share the fixed read view while
	// Close waits for every active build before releasing that view.
	stateMu  sync.RWMutex
	closed   bool
	closeErr error
}

// Open fixes Current Epoch, its ReportSet, and its Entity-vector set once. The
// returned Session never resolves those selections again and must be closed by
// its owner.
func (s *Searcher) Open(ctx context.Context) (*Session, error) {
	if s == nil || s.epochs == nil || s.reports == nil || s.vectors == nil || s.embedder == nil ||
		s.knowledge == nil || s.tokens == nil || s.model == nil || s.sources == nil {
		return nil, querybase.NewInternalFailure(
			errors.New("Local Searcher is not configured"),
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, normalizeFailure(err)
	}
	selected, err := s.epochs.Current(ctx)
	if errors.Is(err, querybase.ErrNoEpoch) {
		return nil, querybase.NewNoPublicationFailure(err)
	}
	if err != nil {
		return nil, normalizePublicationFailure(err)
	}
	return s.OpenAt(ctx, selected)
}

// OpenAt fixes Local evidence to an Epoch already selected by an owning query
// such as DRIFT. It does not resolve Current and loads the ReportSet and Entity
// vector set exactly once for the returned Session.
func (s *Searcher) OpenAt(
	ctx context.Context,
	selected querybase.Epoch,
) (*Session, error) {
	if s == nil || s.epochs == nil || s.reports == nil || s.vectors == nil || s.embedder == nil ||
		s.knowledge == nil || s.tokens == nil || s.model == nil || s.sources == nil {
		return nil, querybase.NewInternalFailure(
			errors.New("Local Searcher is not configured"),
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, normalizeFailure(err)
	}
	reports, err := s.reports.Publication(ctx, selected)
	if err != nil {
		return nil, normalizePublicationFailure(err)
	}
	vectors, err := s.openVectors(ctx, selected.Knowledge.Entities)
	if err != nil {
		return nil, err
	}
	return &Session{searcher: s, epoch: selected, reports: reports, vectors: vectors}, nil
}

// Epoch returns the immutable publication selected for every operation on this
// Session. The returned value is detached from Session state.
func (s *Session) Epoch() querybase.Epoch {
	if s == nil {
		return querybase.Epoch{}
	}
	return s.epoch
}

// ReportView returns an isolated copy of the ReportSet fixed when the Session opened.
func (s *Session) ReportView(ctx context.Context) (queryreport.View, error) {
	if s == nil || s.searcher == nil {
		return queryreport.View{}, querybase.NewInternalFailure(
			errors.New("Local Session is not configured"),
		)
	}
	if err := ctx.Err(); err != nil {
		return queryreport.View{}, normalizeFailure(err)
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if s.closed {
		return queryreport.View{}, querybase.NewInternalFailure(
			errors.New("Local Session is closed"),
		)
	}
	return copyReportView(s.reports), nil
}

// Close releases the fixed Entity-vector view after all active evidence builds
// complete. It is safe to call more than once.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	if s.vectors != nil {
		s.closeErr = s.vectors.Close()
	}
	return s.closeErr
}

// Evidence builds one Local context from the Session's fixed inputs without
// invoking the answer model or final Citation audit. Each call returns fresh
// slices and tables, so DRIFT can retain branch evidence independently.
func (s *Session) Evidence(
	ctx context.Context,
	request SearchRequest,
) (Evidence, error) {
	if err := validateEvidenceRequest(request); err != nil {
		return Evidence{}, querybase.NewInvalidInputFailure(err.Error(), err)
	}
	return s.evidence(ctx, request)
}

// Search generates and audits one Local answer without reopening either fixed
// view. It is useful to callers that need several answers over one stable image.
func (s *Session) Search(ctx context.Context, request SearchRequest) (Result, error) {
	if err := validateSearchRequest(request); err != nil {
		return Result{}, querybase.NewInvalidInputFailure(err.Error(), err)
	}
	evidence, err := s.evidence(ctx, request)
	result := Result{Evidence: evidence}
	if err != nil {
		return result, err
	}
	modelRequest, err := s.searcher.answerRequest(request, evidence)
	if err != nil {
		return result, err
	}
	response, err := s.searcher.model.GenerateAnswer(ctx, modelRequest)
	if err != nil {
		return result, normalizeFailure(err)
	}
	result.Response = response
	return s.searcher.audit(ctx, result)
}

// Stream emits only one final Local answer and audits it against this
// Session's fixed evidence. A terminal error after output is marked partial.
func (s *Session) Stream(
	ctx context.Context,
	request SearchRequest,
	emit querybase.TextDeltaHandler,
) (Result, error) {
	if s == nil || s.searcher == nil || s.searcher.model == nil {
		return Result{}, querybase.NewInternalFailure(
			errors.New("Local Session is not configured"),
		)
	}
	streamer, ok := s.searcher.model.(StreamAnswerModel)
	if !ok {
		return Result{}, querybase.NewInternalFailure(
			errors.New("Local answer model does not support streaming"),
		)
	}
	if emit == nil {
		return Result{}, querybase.NewInvalidInputFailure(
			"Local Search stream handler is required", nil,
		)
	}
	if err := validateSearchRequest(request); err != nil {
		return Result{}, querybase.NewInvalidInputFailure(err.Error(), err)
	}
	evidence, err := s.evidence(ctx, request)
	result := Result{Evidence: evidence}
	if err != nil {
		return result, err
	}
	modelRequest, err := s.searcher.answerRequest(request, evidence)
	if err != nil {
		return result, err
	}
	response, err := streamer.StreamAnswer(ctx, modelRequest, emit)
	result.Response = response
	if err != nil {
		failure := normalizeFailure(err)
		if response != "" {
			failure = querybase.MarkPartialOutput(failure)
		}
		return result, failure
	}
	result, err = s.searcher.audit(ctx, result)
	if err != nil && response != "" {
		return result, querybase.MarkPartialOutput(err)
	}
	return result, err
}

func (s *Session) evidence(
	ctx context.Context,
	request SearchRequest,
) (Evidence, error) {
	if s == nil || s.searcher == nil || s.vectors == nil {
		return Evidence{}, querybase.NewInternalFailure(
			errors.New("Local Session is not configured"),
		)
	}
	if err := ctx.Err(); err != nil {
		return Evidence{}, normalizeFailure(err)
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if s.closed {
		return Evidence{}, querybase.NewInternalFailure(
			errors.New("Local Session is closed"),
		)
	}
	if err := ctx.Err(); err != nil {
		return Evidence{}, normalizeFailure(err)
	}
	return s.searcher.prepareEvidence(
		ctx, s.epoch.Knowledge, request, s.reports, s.vectors,
	)
}

func copyReportView(view queryreport.View) queryreport.View {
	result := view
	result.Communities = make([]queryreport.PublishedCommunity, len(view.Communities))
	for index, community := range view.Communities {
		if community.ParentID != nil {
			parentID := *community.ParentID
			community.ParentID = &parentID
		}
		community.EntityIDs = append([]string(nil), community.EntityIDs...)
		result.Communities[index] = community
	}
	result.Reports = make([]queryreport.PublishedReport, len(view.Reports))
	for index, report := range view.Reports {
		report.Findings = append([]queryreport.ReportFinding(nil), report.Findings...)
		report.Sources.Entities = append([]querybase.KnowledgeReference(nil), report.Sources.Entities...)
		report.Sources.Relations = append([]querybase.KnowledgeReference(nil), report.Sources.Relations...)
		report.Sources.Claims = append([]querybase.ClaimReference(nil), report.Sources.Claims...)
		report.Sources.TextUnitIDs = append([]string(nil), report.Sources.TextUnitIDs...)
		result.Reports[index] = report
	}
	return result
}
