package drift

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

const (
	testHypotheticalPrompt = "Question:{query}\nTemplate:{template}"
	testPrimerPrompt       = "Question:{query}\nReports:{community_reports}"
)

type primerEpochs struct {
	value querybase.Epoch
	err   error
	calls int
}

func (s *primerEpochs) Current(context.Context) (querybase.Epoch, error) {
	s.calls++
	return s.value, s.err
}

type primerReports struct {
	view     queryreport.View
	err      error
	selected []querybase.Epoch
}

func (s *primerReports) Publication(
	_ context.Context,
	selected querybase.Epoch,
) (queryreport.View, error) {
	s.selected = append(s.selected, selected)
	return s.view, s.err
}

type primerVectorStore struct {
	reader ReportVectorReader
	err    error
	ids    []string
}

func (s *primerVectorStore) OpenReports(
	_ context.Context,
	reportSetID string,
) (ReportVectorReader, error) {
	s.ids = append(s.ids, reportSetID)
	return s.reader, s.err
}

type primerVectors struct {
	reportSetID string
	model       string
	dimension   int
	matches     []ReportMatch
	vector      []float64
	limit       int
	reportIDs   []string
	calls       int
	closed      int
}

func (s *primerVectors) ReportSetID() string { return s.reportSetID }
func (s *primerVectors) Model() string       { return s.model }
func (s *primerVectors) Dimension() int      { return s.dimension }
func (s *primerVectors) Search(
	_ context.Context,
	vector []float64,
	limit int,
	reportIDs []string,
) ([]ReportMatch, error) {
	s.calls++
	s.vector = append([]float64(nil), vector...)
	s.limit = limit
	s.reportIDs = append([]string(nil), reportIDs...)
	return append([]ReportMatch(nil), s.matches...), nil
}

func (s *primerVectors) Close() error {
	s.closed++
	return nil
}

type primerEmbedder struct {
	model string
	text  string
	value []float64
}

func (s *primerEmbedder) Model() string { return s.model }
func (s *primerEmbedder) Embed(_ context.Context, text string) ([]float64, error) {
	s.text = text
	return append([]float64(nil), s.value...), nil
}

type primerModel struct {
	hypothetical         string
	mu                   sync.Mutex
	hypotheticalRequests []ModelRequest
	primerRequests       []ModelRequest
}

func (s *primerModel) GenerateHypothetical(
	_ context.Context,
	request ModelRequest,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hypotheticalRequests = append(s.hypotheticalRequests, request)
	return s.hypothetical, nil
}

func (s *primerModel) GeneratePrimer(
	_ context.Context,
	request ModelRequest,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.primerRequests = append(s.primerRequests, request)
	if strings.Contains(request.Prompt, "Report C") {
		return `{"intermediate_answer":"first answer","score":90,"follow_up_queries":["shared","first"]}`, nil
	}
	return `{"intermediate_answer":"second answer","score":70,"follow_up_queries":["shared","second"]}`, nil
}

type primerTokens struct{}

func (primerTokens) Count(_ context.Context, _ string, text string) (int, error) {
	return len([]rune(text)), nil
}

func TestPrimerFixesEpochRetrievesReportsAndPreservesFoldOrder(t *testing.T) {
	epoch := primerEpoch()
	epochs := &primerEpochs{value: epoch}
	reports := &primerReports{view: primerReportView()}
	vectors := &primerVectors{
		reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
		matches: []ReportMatch{
			{ReportID: "report-c", Score: .9},
			{ReportID: "report-a", Score: .8},
			{ReportID: "report-b", Score: .7},
		},
	}
	store := &primerVectorStore{reader: vectors}
	embedder := &primerEmbedder{
		model: "embedding-model", value: []float64{1, 0},
	}
	model := &primerModel{hypothetical: "hypothetical answer"}
	config := DefaultPrimerConfig()
	config.Reports = 3
	config.Folds = 2
	config.HyDEMaxCompletionTokens = 64
	config.PrimerMaxCompletionTokens = 256
	primer := newPrimerForTest(t, epochs, reports, store, embedder, model, config)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "global question"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if epochs.calls != 1 || !reflect.DeepEqual(reports.selected, []querybase.Epoch{epoch}) ||
		!reflect.DeepEqual(store.ids, []string{epoch.ReportSetID, epoch.ReportSetID}) {
		t.Fatalf("fixed reads = Epoch:%d Reports:%#v Vectors:%#v", epochs.calls, reports.selected, store.ids)
	}
	if len(model.hypotheticalRequests) != 1 ||
		model.hypotheticalRequests[0].MaxCompletionTokens != 64 ||
		!strings.Contains(model.hypotheticalRequests[0].Prompt, "Template:Report A") ||
		embedder.text != "hypothetical answer" ||
		!reflect.DeepEqual(vectors.vector, []float64{1, 0}) || vectors.limit != 3 ||
		!reflect.DeepEqual(vectors.reportIDs, []string{"report-a", "report-b", "report-c", "report-d"}) {
		t.Fatalf(
			"hypothetical/embed/search = %#v / %q / %v / %d / %#v",
			model.hypotheticalRequests, embedder.text, vectors.vector, vectors.limit, vectors.reportIDs,
		)
	}
	for _, request := range model.primerRequests {
		if request.MaxCompletionTokens != 256 {
			t.Fatalf("Primer request completion limit = %d", request.MaxCompletionTokens)
		}
	}
	wantReports := []string{"report-c", "report-a", "report-b"}
	gotReports := make([]string, len(result.Reports))
	for index, selected := range result.Reports {
		gotReports[index] = selected.Report.ID
	}
	if !reflect.DeepEqual(gotReports, wantReports) || len(result.Folds) != 2 ||
		!reflect.DeepEqual(result.Folds[0].ReportIDs, []string{"report-c", "report-a"}) ||
		!reflect.DeepEqual(result.Folds[1].ReportIDs, []string{"report-b"}) {
		t.Fatalf("Reports/Folds = %#v / %#v", gotReports, result.Folds)
	}
	if !result.Epoch.Equal(epoch) || result.CommunitySetID != "community-set" ||
		result.Question != "global question" ||
		result.HypotheticalAnswer != "hypothetical answer" ||
		result.IntermediateAnswer != "first answer\n\nsecond answer" || result.Score != 80 ||
		!reflect.DeepEqual(result.FollowUpQueries, []string{"shared", "first", "shared", "second"}) ||
		result.Usage.Calls != 3 {
		t.Fatalf("Primer result = %#v", result)
	}
}

func TestPrimerRestrictsReportSearchToConfiguredCommunityLevel(t *testing.T) {
	epoch := primerEpoch()
	view := primerReportView()
	for index := range view.Communities {
		view.Communities[index].Level = index
	}
	vectors := &primerVectors{
		reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
		matches: []ReportMatch{
			{ReportID: "report-b", Score: .9},
			{ReportID: "report-a", Score: .8},
		},
	}
	config := DefaultPrimerConfig()
	config.CommunityLevel = 1
	config.Reports = 2
	config.Folds = 1
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: view},
		&primerVectorStore{reader: vectors},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		&primerModel{hypothetical: "hypothetical"},
		config,
	)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !reflect.DeepEqual(vectors.reportIDs, []string{"report-a", "report-b"}) ||
		len(result.Reports) != 2 || result.Reports[0].Report.ID != "report-b" ||
		result.Reports[1].Report.ID != "report-a" {
		t.Fatalf("eligible/selected Reports = %#v / %#v", vectors.reportIDs, result.Reports)
	}
}

func TestPrimerFallsBackToQuestionWhenHypotheticalAnswerIsEmpty(t *testing.T) {
	epoch := primerEpoch()
	embedder := &primerEmbedder{model: "embedding-model", value: []float64{1, 0}}
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: primerReportView()},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
			matches: []ReportMatch{{ReportID: "report-a", Score: 1}},
		}},
		embedder,
		&primerModel{hypothetical: " \n"},
		PrimerConfig{
			Reports: 1, Folds: 1, MaxConcurrency: 1,
			MaxPromptTokens: 1000, HyDEMaxCompletionTokens: 1000,
			PrimerMaxCompletionTokens: 1000,
		},
	)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "global question"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result.HypotheticalAnswer != "global question" || embedder.text != "global question" {
		t.Fatalf("fallback = %q / embedded %q", result.HypotheticalAnswer, embedder.text)
	}
}

func TestPrimerRequiresEpochBeforeReadingDerivedData(t *testing.T) {
	epochs := &primerEpochs{err: querybase.ErrNoEpoch}
	reports := &primerReports{}
	store := &primerVectorStore{}
	primer := newPrimerForTest(
		t, epochs, reports, store,
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		&primerModel{}, DefaultPrimerConfig(),
	)

	_, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	assertPrimerFailure(t, err, querybase.FailureNoPublication)
	if epochs.calls != 1 || len(reports.selected) != 0 || len(store.ids) != 0 {
		t.Fatalf("reads = Epoch:%d Reports:%d Vectors:%d", epochs.calls, len(reports.selected), len(store.ids))
	}
}

func TestPrimerRejectsInvalidStructuredResponse(t *testing.T) {
	epoch := primerEpoch()
	invalid := &invalidPrimerModel{hypothetical: "hypothetical"}
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: primerReportView()},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
			matches: []ReportMatch{{ReportID: "report-a", Score: 1}},
		}},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		invalid,
		PrimerConfig{
			Reports: 1, Folds: 1, MaxConcurrency: 1,
			MaxPromptTokens: 1000, HyDEMaxCompletionTokens: 1000,
			PrimerMaxCompletionTokens: 1000,
		},
	)

	_, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	assertPrimerFailure(t, err, querybase.FailureInvalidModelResponse)
}

func TestPrimerReturnsNoEvidenceWithoutStartingFoldCalls(t *testing.T) {
	epoch := primerEpoch()
	model := &primerModel{hypothetical: "hypothetical"}
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: primerReportView()},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
		}},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		model,
		PrimerConfig{
			Reports: 1, Folds: 1, MaxConcurrency: 1,
			MaxPromptTokens: 1000, HyDEMaxCompletionTokens: 1000,
			PrimerMaxCompletionTokens: 1000,
		},
	)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	assertPrimerFailure(t, err, querybase.FailureNoEvidence)
	if !result.Epoch.Equal(epoch) || result.Usage.Calls != 1 || len(model.primerRequests) != 0 {
		t.Fatalf("result/model calls = %#v / %#v", result, model.primerRequests)
	}
}

func TestPrimerRejectsVectorNamespaceOutsideFixedReportSet(t *testing.T) {
	epoch := primerEpoch()
	model := &primerModel{hypothetical: "hypothetical"}
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: primerReportView()},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: "different-report-set", model: "embedding-model", dimension: 2,
		}},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		model,
		DefaultPrimerConfig(),
	)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	assertPrimerFailure(t, err, querybase.FailurePublicationIncomplete)
	if !result.Epoch.Equal(epoch) || len(model.hypotheticalRequests) != 0 || len(model.primerRequests) != 0 {
		t.Fatalf("result/model calls = %#v / %#v / %#v", result, model.hypotheticalRequests, model.primerRequests)
	}
}

func TestPrimerClassifiesEmbeddingDimensionMismatchAsIncompletePublication(t *testing.T) {
	epoch := primerEpoch()
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: primerReportView()},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 3,
		}},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		&primerModel{hypothetical: "hypothetical"},
		DefaultPrimerConfig(),
	)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	assertPrimerFailure(t, err, querybase.FailurePublicationIncomplete)
	if !result.Epoch.Equal(epoch) || result.HypotheticalAnswer != "hypothetical" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPrimerRejectsQuestionThatCannotFitFixedPrompt(t *testing.T) {
	epoch := primerEpoch()
	model := &primerModel{hypothetical: "hypothetical"}
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: primerReportView()},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
		}},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		model,
		PrimerConfig{
			Reports: 1, Folds: 1, MaxConcurrency: 1,
			MaxPromptTokens: 1, HyDEMaxCompletionTokens: 1000,
			PrimerMaxCompletionTokens: 1000,
		},
	)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	assertPrimerFailure(t, err, querybase.FailureInvalidInput)
	if !result.Epoch.Equal(epoch) || len(model.hypotheticalRequests) != 0 || len(model.primerRequests) != 0 {
		t.Fatalf("result/model calls = %#v / %#v / %#v", result, model.hypotheticalRequests, model.primerRequests)
	}
}

func TestPrimerTruncatesTemplateAndReportContentToPromptLimit(t *testing.T) {
	epoch := primerEpoch()
	view := primerReportView()
	view.Reports[0].FullContent = strings.Repeat("Report A evidence ", 20)
	model := &primerModel{hypothetical: "hypothetical"}
	config := DefaultPrimerConfig()
	config.Reports = 1
	config.Folds = 1
	config.MaxPromptTokens = 48
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: view},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
			matches: []ReportMatch{{ReportID: "report-a", Score: 1}},
		}},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		model,
		config,
	)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(model.hypotheticalRequests) != 1 || len(model.primerRequests) != 1 ||
		len([]rune(model.hypotheticalRequests[0].Prompt)) > config.MaxPromptTokens ||
		len([]rune(model.primerRequests[0].Prompt)) > config.MaxPromptTokens ||
		strings.Contains(model.hypotheticalRequests[0].Prompt, view.Reports[0].FullContent) ||
		strings.Contains(model.primerRequests[0].Prompt, view.Reports[0].FullContent) ||
		len(result.Folds) != 1 || !reflect.DeepEqual(result.Folds[0].ReportIDs, []string{"report-a"}) {
		t.Fatalf("truncated Primer = result %#v, hypothetical %#v, folds %#v", result, model.hypotheticalRequests, model.primerRequests)
	}
}

type boundedPrimerModel struct {
	started chan struct{}
	release chan struct{}

	mu        sync.Mutex
	active    int
	maximum   int
	completed int
}

func (*boundedPrimerModel) GenerateHypothetical(context.Context, ModelRequest) (string, error) {
	return "hypothetical", nil
}

func (m *boundedPrimerModel) GeneratePrimer(
	ctx context.Context,
	request ModelRequest,
) (string, error) {
	m.mu.Lock()
	m.active++
	m.maximum = max(m.maximum, m.active)
	m.mu.Unlock()
	m.started <- struct{}{}
	select {
	case <-m.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	m.mu.Lock()
	m.active--
	m.completed++
	m.mu.Unlock()
	answer := "unknown"
	for _, id := range []string{"A", "B", "C", "D"} {
		if strings.Contains(request.Prompt, "Report "+id) {
			answer = id
			break
		}
	}
	return `{"intermediate_answer":"` + answer + `","score":50,"follow_up_queries":["next"]}`, nil
}

func TestPrimerBoundsFoldConcurrencyAndRestoresFoldOrder(t *testing.T) {
	epoch := primerEpoch()
	model := &boundedPrimerModel{
		started: make(chan struct{}, 4),
		release: make(chan struct{}),
	}
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: primerReportView()},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
			matches: []ReportMatch{
				{ReportID: "report-a", Score: 1},
				{ReportID: "report-b", Score: .9},
				{ReportID: "report-c", Score: .8},
				{ReportID: "report-d", Score: .7},
			},
		}},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		model,
		PrimerConfig{
			Reports: 4, Folds: 4, MaxConcurrency: 2,
			MaxPromptTokens: 1000, HyDEMaxCompletionTokens: 1000,
			PrimerMaxCompletionTokens: 1000,
		},
	)
	type outcome struct {
		result PrimerResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
		done <- outcome{result: result, err: err}
	}()
	for range 2 {
		<-model.started
	}
	select {
	case <-model.started:
		t.Fatal("a third fold started before one of the two active folds completed")
	default:
	}
	close(model.release)
	completed := <-done
	if completed.err != nil {
		t.Fatalf("Search() error = %v", completed.err)
	}
	model.mu.Lock()
	maximum, calls := model.maximum, model.completed
	model.mu.Unlock()
	if maximum != 2 || calls != 4 || completed.result.IntermediateAnswer != "A\n\nB\n\nC\n\nD" {
		t.Fatalf(
			"maximum/calls/answer = %d/%d/%q",
			maximum, calls, completed.result.IntermediateAnswer,
		)
	}
}

type invalidPrimerModel struct {
	hypothetical string
}

func (m *invalidPrimerModel) GenerateHypothetical(context.Context, ModelRequest) (string, error) {
	return m.hypothetical, nil
}

func (*invalidPrimerModel) GeneratePrimer(context.Context, ModelRequest) (string, error) {
	return `{"intermediate_answer":"answer","score":101,"follow_up_queries":["next"]}`, nil
}

type correctingPrimerModel struct {
	invalidPrimerModel
	correction PrimerCorrection
}

func (model *correctingPrimerModel) CorrectPrimer(
	_ context.Context,
	correction PrimerCorrection,
) (string, error) {
	model.correction = correction
	return `{"intermediate_answer":"answer","score":80,"follow_up_queries":["next"]}`, nil
}

func TestPrimerReturnsRejectedResultAndReasonForCorrection(t *testing.T) {
	epoch := primerEpoch()
	model := &correctingPrimerModel{invalidPrimerModel: invalidPrimerModel{hypothetical: "hypothetical"}}
	primer := newPrimerForTest(
		t,
		&primerEpochs{value: epoch},
		&primerReports{view: primerReportView()},
		&primerVectorStore{reader: &primerVectors{
			reportSetID: epoch.ReportSetID, model: "embedding-model", dimension: 2,
			matches: []ReportMatch{{ReportID: "report-a", Score: 1}},
		}},
		&primerEmbedder{model: "embedding-model", value: []float64{1, 0}},
		model,
		PrimerConfig{
			Reports: 1, Folds: 1, MaxConcurrency: 1,
			MaxPromptTokens: 1000, HyDEMaxCompletionTokens: 1000,
			PrimerMaxCompletionTokens: 1000,
		},
	)

	result, err := primer.Search(t.Context(), PrimerRequest{Question: "question"})
	if err != nil || len(result.Folds) != 1 || result.Folds[0].Response.Score != 80 {
		t.Fatalf("Search() result/error = %#v/%v", result, err)
	}
	if !strings.Contains(model.correction.Result, `"score":101`) ||
		!strings.Contains(model.correction.Reason, "between 0 and 100") ||
		result.Folds[0].Response.Usage.Calls != 2 {
		t.Fatalf("correction/usage = %#v/%#v", model.correction, result.Folds[0].Response.Usage)
	}
}

func newPrimerForTest(
	t *testing.T,
	epochs querybase.EpochReader,
	reports ReportReader,
	vectors ReportVectorStore,
	embedder Embedder,
	model Model,
	config PrimerConfig,
) *Primer {
	t.Helper()
	primer, err := NewPrimer(
		epochs, reports, vectors, embedder, model, primerTokens{},
		testHypotheticalPrompt, testPrimerPrompt, config,
	)
	if err != nil {
		t.Fatalf("NewPrimer() error = %v", err)
	}
	return primer
}

func primerEpoch() querybase.Epoch {
	return querybase.Epoch{
		ID: 4, CorporaID: "corpora", ReportSetID: "report-set",
	}
}

func primerReportView() queryreport.View {
	report := func(id string, content string) queryreport.PublishedReport {
		return queryreport.PublishedReport{ID: id, CommunityID: "community-" + id, FullContent: content}
	}
	community := func(id string) queryreport.PublishedCommunity {
		return queryreport.PublishedCommunity{ID: "community-" + id}
	}
	return queryreport.View{
		EpochID: 4, ReportSetID: "report-set", CommunitySetID: "community-set", CorporaID: "corpora",
		Communities: []queryreport.PublishedCommunity{
			community("report-a"), community("report-b"),
			community("report-c"), community("report-d"),
		},
		Reports: []queryreport.PublishedReport{
			report("report-a", "Report A"), report("report-b", "Report B"),
			report("report-c", "Report C"), report("report-d", "Report D"),
		},
	}
}

func assertPrimerFailure(t *testing.T, err error, category querybase.FailureCategory) {
	t.Helper()
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != category {
		t.Fatalf("error = %#v, want %q", err, category)
	}
}
