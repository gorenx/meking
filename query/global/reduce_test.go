package global

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	querybase "github.com/memoria-space/meking/query"
)

type reduceModelStub struct {
	request  ReduceModelRequest
	response string
	err      error
	calls    int
}

func (m *reduceModelStub) GenerateReduce(
	_ context.Context,
	request ReduceModelRequest,
) (string, error) {
	m.calls++
	m.request = request
	return m.response, m.err
}

type streamingReduceModelStub struct {
	reduceModelStub
	deltas []string
}

func (m *streamingReduceModelStub) StreamReduce(
	_ context.Context,
	request ReduceModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	m.calls++
	m.request = request
	var response strings.Builder
	for _, delta := range m.deltas {
		if err := emit(delta); err != nil {
			return response.String(), err
		}
		response.WriteString(delta)
	}
	return response.String(), m.err
}

func TestReducerStreamsOnlyFinalAnswerAndCannedNoData(t *testing.T) {
	model := &streamingReduceModelStub{deltas: []string{"final", " answer"}}
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
	var deltas []string
	result, err := reducer.Stream(t.Context(), ReduceRequest{
		Question: "question",
		Map:      mapResultForReduce([]MapPoint{{Description: "point", Score: 1}}),
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || result.Response != "final answer" ||
		!reflect.DeepEqual(deltas, model.deltas) || model.calls != 1 {
		t.Fatalf("result/deltas/calls/error = %#v/%v/%d/%v", result, deltas, model.calls, err)
	}

	deltas = nil
	result, err = reducer.Stream(t.Context(), ReduceRequest{
		Map: mapResultForReduce([]MapPoint{{Description: "none", Score: 0}}),
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || result.Response != NoDataAnswer ||
		!reflect.DeepEqual(deltas, []string{NoDataAnswer}) || model.calls != 1 {
		t.Fatalf("canned result/deltas/calls/error = %#v/%v/%d/%v", result, deltas, model.calls, err)
	}
}

func TestReducerRanksStablyAndStopsAtFirstOverBudgetPoint(t *testing.T) {
	points := []MapPoint{
		{Description: "middle-a", Score: 40, BatchIndex: 0, PointIndex: 0},
		{Description: "ignored", Score: 0, BatchIndex: 0, PointIndex: 1},
		{Description: "highest", Score: 90, BatchIndex: 1, PointIndex: 0},
		{Description: "middle-b", Score: 40, BatchIndex: 2, PointIndex: 0},
	}
	first := formatReducePoint(points[2])
	second := formatReducePoint(points[0])
	model := &reduceModelStub{response: "final [Data: Reports (3)]"}
	reducer, err := NewReducer(
		model,
		codePointCounter{},
		"Reports:\n{report_data}\nFormat: {response_type}\nLimit: {max_length}",
		"knowledge",
		ReduceConfig{
			DataMaxTokens: len([]rune(first)) + len([]rune(second)),
			MaxLength:     321,
			ResponseType:  "configured",
		},
	)
	if err != nil {
		t.Fatalf("NewReducer() error = %v", err)
	}
	mapResult := mapResultForReduce(points)
	result, err := reducer.Reduce(t.Context(), ReduceRequest{
		Question: "question", Map: mapResult, ResponseType: "List of 3 Points",
	})
	if err != nil {
		t.Fatalf("Reduce() error = %v", err)
	}
	wantPoints := []MapPoint{points[2], points[0]}
	if !reflect.DeepEqual(result.ReduceContext.Points, wantPoints) ||
		result.ReduceContext.Text != first+"\n\n"+second ||
		result.ReduceContext.TokenCount != len([]rune(first))+len([]rune(second)) ||
		!result.ReduceContext.Truncated {
		t.Fatalf("Reduce Context = %#v", result.ReduceContext)
	}
	if result.Response != model.response || model.calls != 1 ||
		model.request.UserPrompt != "question" ||
		model.request.SystemPrompt != "Reports:\n"+first+"\n\n"+second+
			"\nFormat: List of 3 Points\nLimit: 321" {
		t.Fatalf("result/request = %#v / %#v", result, model.request)
	}
	if !reflect.DeepEqual(result.Map.Context, mapResult.Context) ||
		!reflect.DeepEqual(result.Map.Batches, mapResult.Batches) {
		t.Fatal("Reduce result did not preserve Map evidence")
	}
}

func TestReducerNoDataAndGeneralKnowledgeBehavior(t *testing.T) {
	model := &reduceModelStub{response: "unexpected"}
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
	result, err := reducer.Reduce(t.Context(), ReduceRequest{
		Map: mapResultForReduce([]MapPoint{{Description: "none", Score: 0}}),
	})
	if err != nil || result.Response != NoDataAnswer || model.calls != 0 ||
		result.ReduceContext.Text != "" {
		t.Fatalf("result/calls/error = %#v/%d/%v", result, model.calls, err)
	}

	model.response = "general answer [LLM: verify]"
	reducer, err = NewReducer(
		model,
		codePointCounter{},
		"Evidence:{report_data}\n{response_type}\n{max_length}",
		"Mark external facts [LLM: verify].",
		ReduceConfig{
			DataMaxTokens: 1, MaxLength: 10, AllowGeneralKnowledge: true,
		},
	)
	if err != nil {
		t.Fatalf("NewReducer(general knowledge) error = %v", err)
	}
	result, err = reducer.Reduce(t.Context(), ReduceRequest{
		Question: "question",
		Map: mapResultForReduce([]MapPoint{{
			Description: "too large", Score: 100, BatchIndex: 0,
		}}),
	})
	if err != nil || result.Response != model.response || model.calls != 1 ||
		result.ReduceContext.Text != "" || !result.ReduceContext.Truncated ||
		!strings.HasSuffix(model.request.SystemPrompt, "\nMark external facts [LLM: verify].") {
		t.Fatalf("general result/request/error = %#v/%#v/%v", result, model.request, err)
	}
}

func TestReducerValidatesConfigurationAndPropagatesFailures(t *testing.T) {
	model := &reduceModelStub{}
	for _, config := range []ReduceConfig{
		{DataMaxTokens: 0, MaxLength: 1},
		{DataMaxTokens: 1, MaxLength: 0},
		{DataMaxTokens: 1, MaxLength: 1, AllowGeneralKnowledge: true},
	} {
		if _, err := NewReducer(model, codePointCounter{}, "prompt", "", config); err == nil {
			t.Fatalf("NewReducer(%+v) error = nil", config)
		}
	}

	want := querybase.NewProviderFailure(true, 429, errors.New("rate limited"))
	model.err = want
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
	input := ReduceRequest{
		Map: mapResultForReduce([]MapPoint{{Description: "point", Score: 1}}),
	}
	if _, err := reducer.Reduce(t.Context(), input); !errors.Is(err, want) {
		t.Fatalf("Reduce(model failure) error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := reducer.Reduce(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("Reduce(canceled) error = %v", err)
	}
}

func mapResultForReduce(points []MapPoint) MapResult {
	contextData := Context{
		ReportSetID: "report-set",
		Sections: []querybase.ContextSection{{
			Name: "Reports",
			Rows: []querybase.ContextRow{{Values: []string{"3"}, InContext: true}},
		}},
	}
	batches := make([]MapBatchResult, 0)
	for _, point := range points {
		for len(batches) <= point.BatchIndex {
			index := len(batches)
			batches = append(batches, MapBatchResult{BatchIndex: index})
		}
		batches[point.BatchIndex].Points = append(batches[point.BatchIndex].Points, point)
	}
	return MapResult{Context: contextData, Batches: batches}
}
