package drift

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
	"github.com/memoria-space/meking/query/qctx"
	queryreport "github.com/memoria-space/meking/query/report"
	querysource "github.com/memoria-space/meking/query/source"
)

type searchPrimerModel struct{}

func (searchPrimerModel) GenerateHypothetical(context.Context, ModelRequest) (string, error) {
	return "hypothetical", nil
}

func (searchPrimerModel) GeneratePrimer(context.Context, ModelRequest) (string, error) {
	return `{"intermediate_answer":"primer answer","score":90,"follow_up_queries":["q1","q2"]}`, nil
}

type searchLocalOpener struct {
	session  LocalSession
	selected []querybase.Epoch
}

func (o *searchLocalOpener) Open(
	_ context.Context,
	selected querybase.Epoch,
) (LocalSession, error) {
	o.selected = append(o.selected, selected)
	return o.session, nil
}

type searchLocalSession struct {
	epoch  querybase.Epoch
	closes int
}

func (s *searchLocalSession) Epoch() querybase.Epoch { return s.epoch }
func (s *searchLocalSession) Close() error           { s.closes++; return nil }

func (s *searchLocalSession) Evidence(
	_ context.Context,
	request querylocal.SearchRequest,
) (querylocal.Evidence, error) {
	reference := querybase.CitationReference{Dataset: querybase.CitationEntities, RecordID: 0}
	table := qctx.Table{
		Text: "entity " + request.Question,
		Rows: []qctx.AcceptedRow{{CitationRecord: querybase.CitationRecord{
			Reference: reference, CorporaID: s.epoch.CorporaID,
			TextUnitIDs: []string{"source-" + request.Question},
		}}},
	}
	return querylocal.Evidence{
		EpochID: s.epoch.ID, ReportSetID: s.epoch.ReportSetID,
		CorporaID: s.epoch.CorporaID,
		Context:   querylocal.Context{Text: table.Text, Entities: table},
	}, nil
}

type searchSources struct {
	err    error
	failAt int
	calls  [][]string
}

func (s *searchSources) Read(
	_ context.Context,
	corporaID string,
	textUnitIDs []string,
) ([]querysource.TextUnitSource, error) {
	s.calls = append(s.calls, append([]string(nil), textUnitIDs...))
	if s.err != nil && (s.failAt == 0 || len(s.calls) == s.failAt) {
		return nil, s.err
	}
	result := make([]querysource.TextUnitSource, len(textUnitIDs))
	for index, id := range textUnitIDs {
		result[index] = querysource.TextUnitSource{
			CorporaID: corporaID, TextUnitID: id, Text: "text " + id,
		}
	}
	return result, nil
}

func TestSearcherAuditsBranchesRenumbersTheirRowsAndAuditsFinalAnswer(t *testing.T) {
	reduceModel := &reduceModelStub{
		response: "combined [Data: Reports (0); Entities (0, 1)]",
	}
	searcher, epochs, opener, sources := newSearcherForTest(t, reduceModel, reduceTestLimits())

	result, err := searcher.Search(t.Context(), SearchRequest{Question: "global question"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if epochs.calls != 1 || !reflect.DeepEqual(opener.selected, []querybase.Epoch{primerEpoch()}) {
		t.Fatalf("fixed selections = Epoch calls %d, Local %#v", epochs.calls, opener.selected)
	}
	if session := opener.session.(*searchLocalSession); session.closes != 1 {
		t.Fatalf("Local Session closes = %d, want 1", session.closes)
	}
	if len(result.Traversal.Branches) != 2 || result.Usage.Calls != 5 {
		t.Fatalf("traversal/usage = %#v / %#v", result.Traversal, result.Usage)
	}
	for index, branch := range result.Traversal.Branches {
		if branch.Status != BranchSucceeded || len(branch.CitationAudit.Items) != 1 ||
			branch.CitationAudit.Items[0].Status != querybase.CitationValid ||
			!strings.Contains(branch.Answer, "Entities (0)") {
			t.Fatalf("branch %d = %#v", index, branch)
		}
	}
	if len(result.ReduceContext.CitationRecords) != 3 ||
		!strings.Contains(result.ReduceContext.Answers[1].Response, "Entities (0)") ||
		!strings.Contains(result.ReduceContext.Answers[2].Response, "Entities (1)") {
		t.Fatalf("Reduce context = %#v", result.ReduceContext)
	}
	if len(result.CitationAudit.Items) != 3 {
		t.Fatalf("final audit = %#v", result.CitationAudit)
	}
	for _, item := range result.CitationAudit.Items {
		if item.Status != querybase.CitationValid {
			t.Fatalf("final Citation = %#v", item)
		}
	}
	if len(sources.calls) != 3 {
		t.Fatalf("source reads = %#v", sources.calls)
	}
}

func TestSearcherKeepsUnknownFinalReferenceAsInvalidAudit(t *testing.T) {
	searcher, _, _, _ := newSearcherForTest(t, &reduceModelStub{
		response: "combined [Data: Entities (99)]",
	}, reduceTestLimits())

	result, err := searcher.Search(t.Context(), SearchRequest{Question: "global question"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.CitationAudit.Items) != 1 ||
		result.CitationAudit.Items[0].InvalidReason != querybase.CitationOutsideContext {
		t.Fatalf("final audit = %#v", result.CitationAudit)
	}
}

func TestSearcherTreatsBranchSourceFailureAsQueryFailure(t *testing.T) {
	reduceModel := &reduceModelStub{response: "unused"}
	searcher, _, _, sources := newSearcherForTest(t, reduceModel, reduceTestLimits())
	sources.err = errors.New("source unavailable")

	result, err := searcher.Search(t.Context(), SearchRequest{Question: "global question"})
	assertDRIFTFailure(t, err, querybase.FailureInternal)
	if result.Response != "" || reduceModel.request != (ReduceModelRequest{}) {
		t.Fatalf("Reduce ran after source failure = %#v / %#v", result, reduceModel.request)
	}
}

func TestSearcherReservesFinalReduceAgainstPrimerAndBranchUsage(t *testing.T) {
	limits := reduceTestLimits()
	limits.ModelCalls = 4
	reduceModel := &reduceModelStub{response: "final answer"}
	searcher, _, _, _ := newSearcherForTest(t, reduceModel, limits)

	result, err := searcher.Search(t.Context(), SearchRequest{Question: "global question"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result.Usage.Calls != 4 || reduceModel.request == (ReduceModelRequest{}) ||
		len(result.Traversal.Branches) != 1 ||
		result.Traversal.Branches[0].Status != BranchSucceeded {
		t.Fatalf("request-wide model budget = %#v / Reduce %#v", result.Usage, reduceModel.request)
	}
}

func TestSearcherStreamsOnlyFinalReduceAndMarksDeliveryFailurePartial(t *testing.T) {
	t.Run("complete", func(t *testing.T) {
		searcher, _, _, _ := newSearcherForTest(t, &reduceModelStub{
			response: "final stream [Data: Reports (0)]",
		}, reduceTestLimits())
		var deltas []string
		result, err := searcher.Stream(
			t.Context(), SearchRequest{Question: "global question"},
			func(delta string) error {
				deltas = append(deltas, delta)
				return nil
			},
		)
		if err != nil || !reflect.DeepEqual(deltas, []string{"final stream [Data: Reports (0)]"}) ||
			result.Response != deltas[0] {
			t.Fatalf("Stream() = %#v, %v, deltas %#v", result, err, deltas)
		}
	})

	t.Run("partial", func(t *testing.T) {
		searcher, _, _, _ := newSearcherForTest(t, &reduceModelStub{
			response: "final prefix",
		}, reduceTestLimits())
		result, err := searcher.Stream(
			t.Context(), SearchRequest{Question: "global question"},
			func(string) error { return errors.New("client stopped") },
		)
		var failure *querybase.Failure
		if !errors.As(err, &failure) || !failure.PartialOutput || result.Response != "final prefix" {
			t.Fatalf("partial Stream() = %#v, %#v", result, err)
		}
	})

	t.Run("final citation audit", func(t *testing.T) {
		searcher, _, _, sources := newSearcherForTest(t, &reduceModelStub{
			response: "final stream [Data: Reports (0)]",
		}, reduceTestLimits())
		sources.err = errors.New("source unavailable")
		sources.failAt = 3
		result, err := searcher.Stream(
			t.Context(), SearchRequest{Question: "global question"}, func(string) error { return nil },
		)
		var failure *querybase.Failure
		if !errors.As(err, &failure) || !failure.PartialOutput ||
			result.Response != "final stream [Data: Reports (0)]" {
			t.Fatalf("final audit failure = %#v, %#v", result, err)
		}
	})
}

type slowPrimerModel struct{}

func (slowPrimerModel) GenerateHypothetical(
	ctx context.Context,
	_ ModelRequest,
) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (slowPrimerModel) GeneratePrimer(context.Context, ModelRequest) (string, error) {
	return "", errors.New("unexpected Primer fold")
}

func TestSearcherRequestDurationIncludesPrimer(t *testing.T) {
	limits := reduceTestLimits()
	limits.Duration = 20 * time.Millisecond
	epoch := primerEpoch()
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: searchReportView()},
		&primerVectorStore{reader: searchReportVectors()},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		slowPrimerModel{},
		searchPrimerConfig(),
	)
	searcher := assembleSearcherForTest(
		t, primer, &searchLocalOpener{}, &reduceModelStub{}, limits,
	)

	result, err := searcher.Search(t.Context(), SearchRequest{Question: "global question"})
	assertDRIFTFailure(t, err, querybase.FailureBudgetExceeded)
	if result.Usage.Calls != 1 || result.Traversal.Usage != result.Usage {
		t.Fatalf("duration usage = %#v", result)
	}
}

func newSearcherForTest(
	t *testing.T,
	reduceModel *reduceModelStub,
	limits RequestLimits,
) (*Searcher, *primerEpochs, *searchLocalOpener, *searchSources) {
	t.Helper()
	epoch := primerEpoch()
	epochReader := &primerEpochs{value: epoch}
	primer := newPrimerForTest(
		t,
		epochReader,
		&primerReports{view: searchReportView()},
		&primerVectorStore{reader: searchReportVectors()},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		searchPrimerModel{},
		searchPrimerConfig(),
	)
	opener := &searchLocalOpener{session: &searchLocalSession{epoch: epoch}}
	sources := &searchSources{}
	return assembleSearcherForTest(t, primer, opener, reduceModel, limits, sources),
		epochReader, opener, sources
}

func assembleSearcherForTest(
	t *testing.T,
	primer *Primer,
	opener *searchLocalOpener,
	reduceModel *reduceModelStub,
	limits RequestLimits,
	optionalSources ...*searchSources,
) *Searcher {
	t.Helper()
	branchModel := &branchModelStub{responses: map[string]string{
		"q1": `{"response":"first [Data: Entities (0)]","score":80,"follow_up_queries":[]}`,
		"q2": `{"response":"second [Data: Entities (0)]","score":70,"follow_up_queries":[]}`,
	}}
	traversalConfig := traversalTestConfig()
	traversalConfig.Limits = limits
	traversal := newTraversalForTest(t, branchModel, branchTokens{}, traversalConfig)
	reduceConfig := DefaultReduceConfig()
	reducer := newReducerForTest(t, reduceModel, reduceConfig, limits)
	sources := &searchSources{}
	if len(optionalSources) > 0 {
		sources = optionalSources[0]
	}
	searcher, err := NewSearcher(primer, opener, traversal, reducer, sources)
	if err != nil {
		t.Fatalf("NewSearcher() error = %v", err)
	}
	return searcher
}

func searchPrimerConfig() PrimerConfig {
	return PrimerConfig{
		Reports: 1, Folds: 1, MaxConcurrency: 1,
		MaxPromptTokens: 1_000, HyDEMaxCompletionTokens: 1_000,
		PrimerMaxCompletionTokens: 1_000,
	}
}

func searchReportView() queryreport.View {
	epoch := primerEpoch()
	return queryreport.View{
		EpochID: epoch.ID, ReportSetID: epoch.ReportSetID,
		CommunitySetID: "community-set", CorporaID: epoch.CorporaID,
		Communities: []queryreport.PublishedCommunity{{ID: "community-a"}},
		Reports: []queryreport.PublishedReport{{
			ID: "report-a", CommunityID: "community-a", FullContent: "Report A",
			Sources: queryreport.ReportSources{TextUnitIDs: []string{"report-source"}},
		}},
	}
}

func searchReportVectors() *primerVectors {
	epoch := primerEpoch()
	return &primerVectors{
		reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
		matches: []ReportMatch{{ReportID: "report-a", Score: 1}},
	}
}

func assertDRIFTFailure(t *testing.T, err error, category querybase.FailureCategory) {
	t.Helper()
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != category {
		t.Fatalf("error = %#v, want %s", err, category)
	}
}
