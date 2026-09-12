package global

import (
	"context"
	"errors"
	"reflect"
	"testing"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
	querysource "github.com/memoria-space/meking/query/source"
)

type reportEvidenceReaderStub struct {
	evidence ReportEvidence
	err      error
	calls    int
	selected []querybase.Epoch
}

type epochReaderStub struct {
	epoch querybase.Epoch
	err   error
	calls int
}

func (r *epochReaderStub) Current(context.Context) (querybase.Epoch, error) {
	r.calls++
	return r.epoch, r.err
}

type searchCitationSources struct {
	corporaIDs  []string
	textUnitIDs [][]string
}

func (s *searchCitationSources) Read(
	_ context.Context,
	corporaID string,
	textUnitIDs []string,
) ([]querysource.TextUnitSource, error) {
	s.corporaIDs = append(s.corporaIDs, corporaID)
	s.textUnitIDs = append(s.textUnitIDs, append([]string(nil), textUnitIDs...))
	result := make([]querysource.TextUnitSource, len(textUnitIDs))
	for index, textUnitID := range textUnitIDs {
		result[index] = querysource.TextUnitSource{
			CorporaID: corporaID, TextUnitID: textUnitID, Text: "evidence",
			DocumentID: "document-" + textUnitID, TextTitle: textUnitID + ".txt",
		}
	}
	return result, nil
}

func (r *reportEvidenceReaderStub) ReportEvidence(
	_ context.Context,
	selected querybase.Epoch,
) (ReportEvidence, error) {
	r.calls++
	r.selected = append(r.selected, selected)
	return r.evidence, r.err
}

type searchModelStub struct {
	mapRequests    []MapModelRequest
	reduceRequests []ReduceModelRequest
	streamDeltas   []string
	mapResponse    string
	reduceResponse string
}

func (m *searchModelStub) GenerateMap(
	_ context.Context,
	request MapModelRequest,
) (string, error) {
	m.mapRequests = append(m.mapRequests, request)
	if m.mapResponse != "" {
		return m.mapResponse, nil
	}
	return `{"points":[{"description":"supported [Data: Reports (0)]","score":90}]}`, nil
}

func (m *searchModelStub) GenerateReduce(
	_ context.Context,
	request ReduceModelRequest,
) (string, error) {
	m.reduceRequests = append(m.reduceRequests, request)
	if m.reduceResponse != "" {
		return m.reduceResponse, nil
	}
	return "answer [Data: Reports (0)]", nil
}

func (m *searchModelStub) StreamReduce(
	_ context.Context,
	request ReduceModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	m.reduceRequests = append(m.reduceRequests, request)
	response := ""
	for _, delta := range m.streamDeltas {
		if err := emit(delta); err != nil {
			return response, err
		}
		response += delta
	}
	return response, nil
}

func TestSearcherFixesOneReportSetForContextMapAndReduce(t *testing.T) {
	reader := &reportEvidenceReaderStub{evidence: searchEvidence()}
	epochs := &epochReaderStub{epoch: searchEpoch()}
	model := &searchModelStub{}
	searcher := newTestSearcher(t, epochs, reader, model)
	config := DefaultContextConfig()
	config.ShuffleData = false
	config.IncludeCommunityWeight = false

	result, err := searcher.Search(t.Context(), SearchRequest{
		Question: "question", ContextConfig: config, ResponseType: "One Paragraph",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if epochs.calls != 1 || reader.calls != 1 ||
		!reflect.DeepEqual(reader.selected, []querybase.Epoch{searchEpoch()}) ||
		result.Context.EpochID != 7 || result.Context.ReportSetID != "report-set" ||
		result.Context.CommunitySetID != "community-set" ||
		result.Context.CorporaID != "corpora" || result.Response == "" ||
		len(result.Map.Batches) != 1 || len(result.ReduceContext.Points) != 1 ||
		len(result.CitationAudit.Items) != 1 ||
		result.CitationAudit.Items[0].Status != "valid" {
		t.Fatalf("result = %#v; Epoch/evidence reads = %d/%d", result, epochs.calls, reader.calls)
	}
	if len(model.mapRequests) != 1 || len(model.reduceRequests) != 1 ||
		model.mapRequests[0].UserPrompt != "question" ||
		model.reduceRequests[0].UserPrompt != "question" {
		t.Fatalf("Map/Reduce requests = %#v / %#v", model.mapRequests, model.reduceRequests)
	}
}

func TestSearcherStreamsOnlyReduceOutput(t *testing.T) {
	reader := &reportEvidenceReaderStub{evidence: searchEvidence()}
	model := &searchModelStub{streamDeltas: []string{"final", " answer"}}
	searcher := newTestSearcher(t, &epochReaderStub{epoch: searchEpoch()}, reader, model)
	config := DefaultContextConfig()
	config.ShuffleData = false
	config.IncludeCommunityWeight = false
	var deltas []string

	result, err := searcher.Stream(t.Context(), SearchRequest{
		Question: "question", ContextConfig: config,
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || result.Response != "final answer" ||
		!reflect.DeepEqual(deltas, model.streamDeltas) || reader.calls != 1 ||
		len(model.mapRequests) != 1 || len(model.reduceRequests) != 1 {
		t.Fatalf("result/deltas/reads/error = %#v/%v/%d/%v", result, deltas, reader.calls, err)
	}
}

func TestSearcherRequiresPublishedEpochBeforeReadingEvidence(t *testing.T) {
	epochs := &epochReaderStub{err: querybase.ErrNoEpoch}
	reader := &reportEvidenceReaderStub{evidence: searchEvidence()}
	searcher := newTestSearcher(t, epochs, reader, &searchModelStub{})

	_, err := searcher.Search(t.Context(), SearchRequest{Question: "question"})
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != querybase.FailureNoPublication {
		t.Fatalf("Search() error = %#v", err)
	}
	if epochs.calls != 1 || reader.calls != 0 {
		t.Fatalf("Epoch/evidence reads = %d/%d", epochs.calls, reader.calls)
	}
}

func TestSearcherAuditsOnlyReportsCitedByReduceAcceptedPoints(t *testing.T) {
	reader := &reportEvidenceReaderStub{evidence: searchEvidenceWithTwoReports()}
	model := &searchModelStub{
		mapResponse: `{"points":[` +
			`{"description":"accepted [Data: Reports (0)]","score":90},` +
			`{"description":"trimmed [Data: Reports (1)]","score":80}` +
			`]}`,
		reduceResponse: "answer [Data: Reports (0, 1)]",
	}
	contextBuilder, err := NewContextBuilder(codePointCounter{}, nil)
	if err != nil {
		t.Fatalf("NewContextBuilder() error = %v", err)
	}
	mapper, err := NewMapper(model, "{context_data}", DefaultMapConfig())
	if err != nil {
		t.Fatalf("NewMapper() error = %v", err)
	}
	accepted := MapPoint{
		Description: "accepted [Data: Reports (0)]", Score: 90,
	}
	reducer, err := NewReducer(
		model,
		codePointCounter{},
		"{report_data} {response_type} {max_length}",
		"",
		ReduceConfig{
			DataMaxTokens: len([]rune(formatReducePoint(accepted))),
			MaxLength:     100,
		},
	)
	if err != nil {
		t.Fatalf("NewReducer() error = %v", err)
	}
	sources := &searchCitationSources{}
	searcher, err := NewSearcher(
		&epochReaderStub{epoch: searchEpoch()}, reader, contextBuilder, mapper, reducer, sources,
	)
	if err != nil {
		t.Fatalf("NewSearcher() error = %v", err)
	}
	config := DefaultContextConfig()
	config.ShuffleData = false
	config.IncludeCommunityWeight = false
	config.IncludeCommunityRank = false

	result, err := searcher.Search(t.Context(), SearchRequest{
		Question: "question", ContextConfig: config,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.ReduceContext.Points) != 1 || !result.ReduceContext.Truncated ||
		!reflect.DeepEqual(result.ReduceContext.Points[0].ReportIDs, []int{0}) {
		t.Fatalf("Reduce Context = %#v", result.ReduceContext)
	}
	if len(result.CitationAudit.Items) != 2 ||
		result.CitationAudit.Items[0].Status != querybase.CitationValid ||
		result.CitationAudit.Items[1].Status != querybase.CitationInvalid ||
		result.CitationAudit.Items[1].InvalidReason != querybase.CitationOutsideContext {
		t.Fatalf("Citation audit = %#v", result.CitationAudit)
	}
	if !reflect.DeepEqual(sources.corporaIDs, []string{"corpora"}) ||
		!reflect.DeepEqual(sources.textUnitIDs, [][]string{{"unit-0"}}) {
		t.Fatalf("source reads = %#v / %#v", sources.corporaIDs, sources.textUnitIDs)
	}
}

func newTestSearcher(
	t *testing.T,
	epochs querybase.EpochReader,
	reader ReportEvidenceReader,
	model *searchModelStub,
) *Searcher {
	t.Helper()
	contextBuilder, err := NewContextBuilder(codePointCounter{}, nil)
	if err != nil {
		t.Fatalf("NewContextBuilder() error = %v", err)
	}
	mapper, err := NewMapper(model, "{context_data} {max_length}", DefaultMapConfig())
	if err != nil {
		t.Fatalf("NewMapper() error = %v", err)
	}
	reducer, err := NewReducer(
		model,
		codePointCounter{},
		"{report_data} {response_type} {max_length}",
		"",
		DefaultReduceConfig(),
	)
	if err != nil {
		t.Fatalf("NewReducer() error = %v", err)
	}
	searcher, err := NewSearcher(
		epochs,
		reader,
		contextBuilder,
		mapper,
		reducer,
		&searchCitationSources{},
	)
	if err != nil {
		t.Fatalf("NewSearcher() error = %v", err)
	}
	return searcher
}

func searchEvidence() ReportEvidence {
	return ReportEvidence{ReportView: queryreport.View{
		EpochID:     7,
		ReportSetID: "report-set", CommunitySetID: "community-set", CorporaID: "corpora",
		Communities: []queryreport.PublishedCommunity{{
			ID: "community", Number: 0, Level: 0, EntityIDs: []string{"entity"},
		}},
		Reports: []queryreport.PublishedReport{{
			ID: "report", CommunityID: "community", Title: "title", Summary: "summary",
			FullContent: "content", Rank: 1,
			Sources: queryreport.ReportSources{
				Entities:    []querybase.KnowledgeReference{{ID: "entity", Version: 1}},
				TextUnitIDs: []string{"unit"},
			},
		}},
	}}
}

func searchEvidenceWithTwoReports() ReportEvidence {
	return ReportEvidence{ReportView: queryreport.View{
		EpochID:     7,
		ReportSetID: "report-set", CommunitySetID: "community-set", CorporaID: "corpora",
		Communities: []queryreport.PublishedCommunity{
			{ID: "community-0", Number: 0, Level: 0, EntityIDs: []string{"entity-0"}},
			{ID: "community-1", Number: 1, Level: 0, EntityIDs: []string{"entity-1"}},
		},
		Reports: []queryreport.PublishedReport{
			{
				ID: "report-0", CommunityID: "community-0", Title: "title 0",
				Summary: "summary 0", FullContent: "content 0", Rank: 1,
				Sources: queryreport.ReportSources{TextUnitIDs: []string{"unit-0"}},
			},
			{
				ID: "report-1", CommunityID: "community-1", Title: "title 1",
				Summary: "summary 1", FullContent: "content 1", Rank: 1,
				Sources: queryreport.ReportSources{TextUnitIDs: []string{"unit-1"}},
			},
		},
	}}
}

func searchEpoch() querybase.Epoch {
	return querybase.Epoch{
		ID: 7, CorporaID: "corpora", ReportSetID: "report-set",
	}
}
