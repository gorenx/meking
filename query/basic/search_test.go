package basic_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	querybase "github.com/memoria-space/meking/query"
	querybasic "github.com/memoria-space/meking/query/basic"
	querysource "github.com/memoria-space/meking/query/source"
)

const (
	testCorpus = "10000000-0000-4000-8000-000000000001"
	newCorpus  = "10000000-0000-4000-8000-000000000002"
	testPrompt = "Evidence:\n{context_data}\nFormat: {response_type}"
)

func TestSearcherUsesFixedCorpusOrderAndAuditsAcceptedRows(t *testing.T) {
	epochs := &epochStub{values: []querybase.Epoch{testEpoch()}}
	reader := &textUnitVectorReaderStub{
		corporaID: testCorpus,
		model:     "embedding-model",
		dimension: 2,
		matches: []querybasic.TextUnitMatch{
			{TextUnitID: "unit-b", Score: 0.9},
			{TextUnitID: "unit-a", Score: 0.8},
			{TextUnitID: "unit-b", Score: 0.7},
		},
	}
	sources := &sourceStub{values: []querysource.TextUnitSource{
		{CorporaID: testCorpus, TextUnitID: "unit-a", Text: "alpha"},
		{CorporaID: testCorpus, TextUnitID: "unit-a", Text: "alpha"},
		{CorporaID: testCorpus, TextUnitID: "unit-b", Text: "beta"},
	}}
	model := &answerModelStub{response: "Answer [Data: Sources (0, 1)]"}
	searcher := mustSearcher(t, epochs, vectorStoreStub{
		open: func(_ context.Context, id string) (querybasic.TextUnitVectorReader, error) {
			if id != testCorpus {
				t.Fatalf("OpenTextUnits ID = %q", id)
			}
			return reader, nil
		},
	}, model, sources, querybasic.DefaultConfig())

	result, err := searcher.Search(t.Context(), querybasic.SearchRequest{
		Question:     "original question",
		ResponseType: "List",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if epochs.calls != 1 || result.EpochID != 3 || result.CorporaID != testCorpus {
		t.Fatalf("fixed Epoch/Corpus = %d/%q, calls = %d", result.EpochID, result.CorporaID, epochs.calls)
	}
	if result.Context.Text != "id|text\n0|alpha\n1|beta\n" || len(result.Context.Rows) != 2 {
		t.Fatalf("Context = %#v", result.Context)
	}
	if !strings.Contains(model.request.SystemPrompt, result.Context.Text) ||
		!strings.Contains(model.request.SystemPrompt, "Format: List") ||
		model.request.UserPrompt != "original question" {
		t.Fatalf("model request = %#v", model.request)
	}
	if !reflect.DeepEqual(reader.vector, []float64{1, 0}) || reader.limit != querybasic.DefaultTopK {
		t.Fatalf("vector request = %v, limit %d", reader.vector, reader.limit)
	}
	if len(result.Matches) != 3 || len(result.CitationAudit.Items) != 2 ||
		result.CitationAudit.Items[0].Status != querybase.CitationValid ||
		len(result.CitationAudit.Items[0].Sources) != 2 ||
		result.CitationAudit.Items[1].Status != querybase.CitationValid {
		t.Fatalf("result = %#v", result)
	}
	if len(sources.requests) != 2 ||
		!reflect.DeepEqual(sources.requests[0], []string{"unit-b", "unit-a"}) {
		t.Fatalf("source requests = %#v", sources.requests)
	}
}

func TestSearcherDoesNotSwitchEpochWhenExactVectorsAreMissing(t *testing.T) {
	epochs := &epochStub{values: []querybase.Epoch{testEpoch(), {
		ID: 4, CorporaID: newCorpus, ReportSetID: "report-set-4",
	}}}
	opened := make([]string, 0, 1)
	searcher := mustSearcher(t, epochs, vectorStoreStub{
		open: func(_ context.Context, id string) (querybasic.TextUnitVectorReader, error) {
			opened = append(opened, id)
			return nil, errors.New("missing exact vectors")
		},
	}, &answerModelStub{response: "answer"}, &sourceStub{}, querybasic.DefaultConfig())

	result, err := searcher.Search(t.Context(), querybasic.SearchRequest{Question: "question"})
	assertFailureCategory(t, err, querybase.FailurePublicationIncomplete)
	if epochs.calls != 1 || result.EpochID != 3 || result.CorporaID != testCorpus ||
		!reflect.DeepEqual(opened, []string{testCorpus}) {
		t.Fatalf("result/opened/error = %#v / %v / %v", result, opened, err)
	}
}

func TestSearcherReturnsDistinctPreAnswerFailures(t *testing.T) {
	tests := []struct {
		name     string
		epochs   *epochStub
		store    vectorStoreStub
		config   querybasic.Config
		question string
		category querybase.FailureCategory
	}{
		{
			name: "invalid question", epochs: &epochStub{values: []querybase.Epoch{testEpoch()}},
			store: validVectorStore(nil), config: querybasic.DefaultConfig(), question: "  ",
			category: querybase.FailureInvalidInput,
		},
		{
			name: "no publication", epochs: &epochStub{errs: []error{querybase.ErrNoEpoch}},
			store: validVectorStore(nil), config: querybasic.DefaultConfig(), question: "question",
			category: querybase.FailureNoPublication,
		},
		{
			name: "missing vector Namespace", epochs: &epochStub{values: []querybase.Epoch{testEpoch()}},
			store: vectorStoreStub{open: func(context.Context, string) (querybasic.TextUnitVectorReader, error) {
				return nil, errors.New("missing")
			}}, config: querybasic.DefaultConfig(), question: "question",
			category: querybase.FailurePublicationIncomplete,
		},
		{
			name: "model mismatch", epochs: &epochStub{values: []querybase.Epoch{testEpoch()}},
			store: vectorStoreStub{open: func(context.Context, string) (querybasic.TextUnitVectorReader, error) {
				return &textUnitVectorReaderStub{corporaID: testCorpus, model: "other", dimension: 2}, nil
			}}, config: querybasic.DefaultConfig(), question: "question",
			category: querybase.FailurePublicationIncomplete,
		},
		{
			name: "no vector matches", epochs: &epochStub{values: []querybase.Epoch{testEpoch()}},
			store: validVectorStore(nil), config: querybasic.DefaultConfig(), question: "question",
			category: querybase.FailureNoEvidence,
		},
		{
			name: "first source exceeds budget", epochs: &epochStub{values: []querybase.Epoch{testEpoch()}},
			store: validVectorStore([]querybasic.TextUnitMatch{{TextUnitID: "unit"}}),
			config: func() querybasic.Config {
				config := querybasic.DefaultConfig()
				config.MaxContextTokens = utf8.RuneCountInString("id|text\n")
				return config
			}(), question: "question", category: querybase.FailureBudgetExceeded,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &answerModelStub{response: "answer"}
			searcher := mustSearcher(t, test.epochs, test.store, model, &sourceStub{values: []querysource.TextUnitSource{{
				CorporaID: testCorpus, TextUnitID: "unit", Text: "source",
			}}}, test.config)
			_, err := searcher.Search(t.Context(), querybasic.SearchRequest{Question: test.question})
			assertFailureCategory(t, err, test.category)
			if model.calls != 0 {
				t.Fatalf("answer model calls = %d", model.calls)
			}
		})
	}
}

func TestSearcherMarksPartialStreamingFailure(t *testing.T) {
	modelErr := errors.New("stream failed")
	model := &answerModelStub{streamResponse: "partial", streamErr: modelErr}
	searcher := mustSearcher(
		t,
		&epochStub{values: []querybase.Epoch{testEpoch()}},
		validVectorStore([]querybasic.TextUnitMatch{{TextUnitID: "unit"}}),
		model,
		&sourceStub{values: []querysource.TextUnitSource{{
			CorporaID: testCorpus, TextUnitID: "unit", Text: "source",
		}}},
		querybasic.DefaultConfig(),
	)
	deltas := ""
	result, err := searcher.Stream(
		t.Context(),
		querybasic.SearchRequest{Question: "question"},
		func(delta string) error { deltas += delta; return nil },
	)
	var failure *querybase.Failure
	if !errors.As(err, &failure) || !failure.PartialOutput ||
		!errors.Is(err, modelErr) || result.Response != "partial" || deltas != "partial" {
		t.Fatalf("result/deltas/error = %#v / %q / %#v", result, deltas, err)
	}
}

func TestNewSearcherRequiresBothPromptFields(t *testing.T) {
	_, err := querybasic.NewSearcher(
		&epochStub{},
		validVectorStore(nil),
		embedderStub{},
		runeCounter{},
		&answerModelStub{},
		&sourceStub{},
		"Only {context_data}",
		querybasic.DefaultConfig(),
	)
	if err == nil {
		t.Fatal("NewSearcher() accepted a prompt without response_type")
	}
}

func mustSearcher(
	t *testing.T,
	epochs querybase.EpochReader,
	vectors querybasic.VectorStore,
	model querybasic.AnswerModel,
	sources querysource.Reader,
	config querybasic.Config,
) *querybasic.Searcher {
	t.Helper()
	searcher, err := querybasic.NewSearcher(
		epochs,
		vectors,
		embedderStub{},
		runeCounter{},
		model,
		sources,
		testPrompt,
		config,
	)
	if err != nil {
		t.Fatalf("NewSearcher() error = %v", err)
	}
	return searcher
}

func validVectorStore(matches []querybasic.TextUnitMatch) vectorStoreStub {
	return vectorStoreStub{open: func(_ context.Context, id string) (querybasic.TextUnitVectorReader, error) {
		return &textUnitVectorReaderStub{
			corporaID: id,
			model:     "embedding-model",
			dimension: 2,
			matches:   append([]querybasic.TextUnitMatch(nil), matches...),
		}, nil
	}}
}

func assertFailureCategory(t *testing.T, err error, category querybase.FailureCategory) {
	t.Helper()
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != category {
		t.Fatalf("error = %#v, want category %q", err, category)
	}
}

type epochStub struct {
	values []querybase.Epoch
	errs   []error
	calls  int
}

func (s *epochStub) Current(context.Context) (querybase.Epoch, error) {
	index := s.calls
	s.calls++
	if index < len(s.errs) && s.errs[index] != nil {
		return querybase.Epoch{}, s.errs[index]
	}
	if index >= len(s.values) {
		return querybase.Epoch{}, errors.New("unexpected Epoch Current call")
	}
	return s.values[index], nil
}

func testEpoch() querybase.Epoch {
	return querybase.Epoch{
		ID: 3, CorporaID: testCorpus, ReportSetID: "report-set-3",
	}
}

type vectorStoreStub struct {
	open func(context.Context, string) (querybasic.TextUnitVectorReader, error)
}

func (s vectorStoreStub) OpenTextUnits(
	ctx context.Context,
	id string,
) (querybasic.TextUnitVectorReader, error) {
	return s.open(ctx, id)
}

type textUnitVectorReaderStub struct {
	corporaID string
	model     string
	dimension int
	matches   []querybasic.TextUnitMatch
	vector    []float64
	limit     int
	closed    int
}

func (s *textUnitVectorReaderStub) CorporaID() string { return s.corporaID }
func (s *textUnitVectorReaderStub) Model() string     { return s.model }
func (s *textUnitVectorReaderStub) Dimension() int    { return s.dimension }
func (s *textUnitVectorReaderStub) Search(
	_ context.Context,
	vector []float64,
	limit int,
) ([]querybasic.TextUnitMatch, error) {
	s.vector = append([]float64(nil), vector...)
	s.limit = limit
	return append([]querybasic.TextUnitMatch(nil), s.matches...), nil
}

func (s *textUnitVectorReaderStub) Close() error {
	s.closed++
	return nil
}

type embedderStub struct{}

func (embedderStub) Model() string { return "embedding-model" }
func (embedderStub) EmbedQuestion(context.Context, string) ([]float64, error) {
	return []float64{1, 0}, nil
}

type runeCounter struct{}

func (runeCounter) Count(_ context.Context, _ string, text string) (int, error) {
	return utf8.RuneCountInString(text), nil
}

type answerModelStub struct {
	request        querybasic.AnswerModelRequest
	response       string
	streamResponse string
	streamErr      error
	calls          int
}

func (s *answerModelStub) GenerateAnswer(
	_ context.Context,
	request querybasic.AnswerModelRequest,
) (string, error) {
	s.calls++
	s.request = request
	return s.response, nil
}

func (s *answerModelStub) StreamAnswer(
	_ context.Context,
	request querybasic.AnswerModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	s.calls++
	s.request = request
	if s.streamResponse != "" {
		if err := emit(s.streamResponse); err != nil {
			return s.streamResponse, err
		}
	}
	return s.streamResponse, s.streamErr
}

type sourceStub struct {
	values   []querysource.TextUnitSource
	err      error
	requests [][]string
}

func (s *sourceStub) Read(
	_ context.Context,
	_ string,
	ids []string,
) ([]querysource.TextUnitSource, error) {
	s.requests = append(s.requests, append([]string(nil), ids...))
	return append([]querysource.TextUnitSource(nil), s.values...), s.err
}
