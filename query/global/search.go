package global

import (
	"context"
	"errors"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	querycitation "github.com/memoria-space/meking/query/internal/citation"
	querysource "github.com/memoria-space/meking/query/source"
)

// SearchRequest contains the question, conversation, selection policy, and
// presentation choices for one ReportSet-fixed Global Search execution.
type SearchRequest struct {
	// Question is sent unchanged to selection, Map, and Reduce models.
	Question string
	// Conversation is rendered into every Map ContextChunk after request validation.
	Conversation []querybase.ConversationTurn
	// ContextConfig controls Report selection and Map evidence allocation.
	ContextConfig ContextConfig
	// ResponseType overrides the Reducer's configured final format when non-empty.
	ResponseType string
}

// Searcher executes one complete Epoch-backed Context, Map, and Reduce flow.
// It fixes Current once per call and passes that immutable selection to every
// derived-data reader; later publication affects only subsequent calls.
type Searcher struct {
	// epochs fixes the unified publication once at request start.
	epochs querybase.EpochReader
	// evidence reads one exact ReportSet and its exact Knowledge versions.
	evidence ReportEvidenceReader
	// context selects and renders the request-local Reports table.
	context *ContextBuilder
	// mapper evaluates each token-bounded Reports table chunk.
	mapper *Mapper
	// reducer produces the final response from ranked Map conclusions.
	reducer *Reducer
	// sources resolves TextUnit occurrences only after Global fixes its final
	// Citation scope from the completed Reduce input.
	sources querysource.Reader
}

// NewSearcher creates the provider-independent Global Search application use case.
func NewSearcher(
	epochs querybase.EpochReader,
	evidence ReportEvidenceReader,
	contextBuilder *ContextBuilder,
	mapper *Mapper,
	reducer *Reducer,
	sources querysource.Reader,
) (*Searcher, error) {
	if epochs == nil {
		return nil, errors.New("create Global Searcher: Epoch reader is required")
	}
	if evidence == nil {
		return nil, errors.New("create Global Searcher: ReportEvidenceReader is required")
	}
	if contextBuilder == nil {
		return nil, errors.New("create Global Searcher: ContextBuilder is required")
	}
	if mapper == nil {
		return nil, errors.New("create Global Searcher: Mapper is required")
	}
	if reducer == nil {
		return nil, errors.New("create Global Searcher: Reducer is required")
	}
	if sources == nil {
		return nil, errors.New("create Global Searcher: SourceReader is required")
	}
	return &Searcher{
		epochs: epochs, evidence: evidence, context: contextBuilder, mapper: mapper, reducer: reducer,
		sources: sources,
	}, nil
}

// Search fixes Current Epoch and its Report evidence once, executes Map, and
// returns the final Reduce answer with its complete in-memory audit trail.
func (s *Searcher) Search(ctx context.Context, request SearchRequest) (SearchResult, error) {
	mapResult, err := s.mapQuestion(ctx, request)
	if err != nil {
		return SearchResult{}, err
	}
	result, err := s.reducer.Reduce(ctx, ReduceRequest{
		Question:     request.Question,
		Map:          mapResult,
		ResponseType: request.ResponseType,
	})
	if err != nil {
		return result, err
	}
	return s.audit(ctx, result)
}

// Stream performs the same fixed-evidence and Map phases as Search, then emits
// only final Reduce text through emit.
func (s *Searcher) Stream(
	ctx context.Context,
	request SearchRequest,
	emit querybase.TextDeltaHandler,
) (SearchResult, error) {
	mapResult, err := s.mapQuestion(ctx, request)
	if err != nil {
		return SearchResult{}, err
	}
	result, err := s.reducer.Stream(ctx, ReduceRequest{
		Question:     request.Question,
		Map:          mapResult,
		ResponseType: request.ResponseType,
	}, emit)
	if err != nil {
		return result, err
	}
	return s.audit(ctx, result)
}

func (s *Searcher) mapQuestion(
	ctx context.Context,
	request SearchRequest,
) (MapResult, error) {
	if s == nil || s.epochs == nil || s.evidence == nil || s.context == nil || s.mapper == nil ||
		s.reducer == nil || s.sources == nil {
		return MapResult{}, errors.New("Global Searcher is not configured")
	}
	if strings.TrimSpace(request.Question) == "" {
		return MapResult{}, querybase.NewInvalidInputFailure(
			"Global Search question is required",
			nil,
		)
	}
	if err := ctx.Err(); err != nil {
		return MapResult{}, err
	}
	selected, err := s.epochs.Current(ctx)
	if errors.Is(err, querybase.ErrNoEpoch) {
		return MapResult{}, querybase.NewNoPublicationFailure(err)
	}
	if err != nil {
		return MapResult{}, err
	}
	evidence, err := s.evidence.ReportEvidence(ctx, selected)
	if err != nil {
		return MapResult{}, err
	}
	contextData, err := s.context.Build(ctx, ContextRequest{
		Evidence:     evidence,
		Question:     request.Question,
		Conversation: append([]querybase.ConversationTurn(nil), request.Conversation...),
		Config:       request.ContextConfig,
	})
	if err != nil {
		return MapResult{}, err
	}
	return s.mapper.Map(ctx, MapRequest{Question: request.Question, Context: contextData})
}

func (s *Searcher) audit(ctx context.Context, result SearchResult) (SearchResult, error) {
	records, err := acceptedReportCitationRecords(result)
	if err != nil {
		return result, err
	}
	audit, err := querycitation.Audit(ctx, result.Response, records, s.sources)
	if err != nil {
		return result, err
	}
	result.CitationAudit = audit
	return result, nil
}

func acceptedReportCitationRecords(result SearchResult) ([]querybase.CitationRecord, error) {
	reports := make(map[int]ReportReference, len(result.Context.Reports))
	for _, report := range result.Context.Reports {
		if report.RecordID < 0 {
			return nil, errors.New("Global Context contains a negative Report record ID")
		}
		if _, duplicate := reports[report.RecordID]; duplicate {
			return nil, errors.New("Global Context contains a duplicate Report record ID")
		}
		reports[report.RecordID] = report
	}

	seen := make(map[int]struct{})
	records := make([]querybase.CitationRecord, 0)
	for _, point := range result.ReduceContext.Points {
		for _, reportID := range point.ReportIDs {
			if _, duplicate := seen[reportID]; duplicate {
				continue
			}
			report, exists := reports[reportID]
			if !exists {
				return nil, errors.New("Global Reduce point references an unknown Report record ID")
			}
			seen[reportID] = struct{}{}
			records = append(records, querybase.CitationRecord{
				Reference: querybase.CitationReference{
					Dataset:  querybase.CitationReports,
					RecordID: report.RecordID,
				},
				CorporaID:   result.Context.CorporaID,
				TextUnitIDs: append([]string(nil), report.TextUnitIDs...),
			})
		}
	}
	return records, nil
}
