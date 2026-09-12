package global

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	querybase "github.com/memoria-space/meking/query"
)

// concurrentMapModel exposes active calls so the test can hold one complete
// frontier and verify the configured concurrency bound.
type concurrentMapModel struct {
	started chan string
	release chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
	calls   atomic.Int32
}

func (m *concurrentMapModel) GenerateMap(
	ctx context.Context,
	request MapModelRequest,
) (string, error) {
	m.calls.Add(1)
	active := m.active.Add(1)
	defer m.active.Add(-1)
	for {
		maximum := m.maximum.Load()
		if active <= maximum || m.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	m.started <- request.SystemPrompt
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-m.release:
	}
	switch {
	case strings.Contains(request.SystemPrompt, "batch-1"):
		return "", querybase.NewProviderFailure(true, 429, errors.New("rate limited"))
	case strings.Contains(request.SystemPrompt, "batch-2"):
		return "not json", nil
	case strings.Contains(request.SystemPrompt, "batch-3"):
		return `{"points":[{"description":"highest [Data: Reports (3)]","score":9},{"description":"zero","score":0}]}`, nil
	default:
		return `{"points":[{"description":"lower [Data: Reports (0)]","score":5}]}`, nil
	}
}

func TestMapperBoundsConcurrencyAndIsolatesBatchFailures(t *testing.T) {
	model := &concurrentMapModel{
		started: make(chan string, 4),
		release: make(chan struct{}),
	}
	mapper, err := NewMapper(model, "{context_data} {max_length}", MapConfig{
		MaxLength: 1000, MaxConcurrency: 2,
	})
	if err != nil {
		t.Fatalf("NewMapper() error = %v", err)
	}
	contextData := Context{ReportSetID: "report-set"}
	for index := 0; index < 4; index++ {
		contextData.Chunks = append(contextData.Chunks, ContextChunk{
			Index: index, Text: "batch-" + string(rune('0'+index)), ReportIDs: []int{index},
		})
	}
	done := make(chan struct{})
	var result MapResult
	var mapErr error
	go func() {
		defer close(done)
		result, mapErr = mapper.Map(t.Context(), MapRequest{
			Question: "question", Context: contextData,
		})
	}()
	for index := 0; index < 2; index++ {
		select {
		case <-model.started:
		case <-time.After(time.Second):
			t.Fatal("Map call did not start")
		}
	}
	select {
	case prompt := <-model.started:
		t.Fatalf("third Map call started above limit: %q", prompt)
	case <-time.After(25 * time.Millisecond):
	}
	close(model.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Map() did not finish")
	}
	if mapErr != nil {
		t.Fatalf("Map() error = %v", mapErr)
	}
	if model.maximum.Load() != 2 || model.calls.Load() != 4 {
		t.Fatalf("model maximum/calls = %d/%d, want 2/4", model.maximum.Load(), model.calls.Load())
	}
	if len(result.Batches) != 4 ||
		result.Batches[0].Status != MapBatchSucceeded ||
		result.Batches[1].Status != MapBatchModelFailure ||
		result.Batches[2].Status != MapBatchInvalidResponse ||
		result.Batches[3].Status != MapBatchSucceeded {
		t.Fatalf("batch results = %#v", result.Batches)
	}
	provider := result.Batches[1].Failure
	if provider == nil || provider.Category != querybase.FailureProviderRetryable ||
		provider.StatusCode != 429 || !provider.Retryable {
		t.Fatalf("provider batch failure = %#v", provider)
	}
	invalid := result.Batches[2].Failure
	if invalid == nil || invalid.Category != querybase.FailureInvalidModelResponse ||
		len(result.Batches[2].Points) != 1 || result.Batches[2].Points[0].Score != 0 {
		t.Fatalf("invalid response batch = %#v", result.Batches[2])
	}
	ranked := result.RankedPoints()
	if len(ranked) != 2 ||
		ranked[0].Description != "highest [Data: Reports (3)]" || ranked[0].Score != 9 ||
		!reflect.DeepEqual(ranked[0].ReportIDs, []int{3}) ||
		ranked[1].Description != "lower [Data: Reports (0)]" || ranked[1].Score != 5 ||
		!reflect.DeepEqual(ranked[1].ReportIDs, []int{0}) {
		t.Fatalf("ranked points = %#v", ranked)
	}
	if result.Context.ReportSetID != "report-set" {
		t.Fatalf("fixed Context = %#v", result.Context)
	}
}

type citationMapModel struct {
	response string
}

type correctingMapModel struct {
	correction MapCorrection
}

func (*correctingMapModel) GenerateMap(context.Context, MapModelRequest) (string, error) {
	return "not-json", nil
}

func (model *correctingMapModel) CorrectMap(
	_ context.Context,
	correction MapCorrection,
) (string, error) {
	model.correction = correction
	return `{"points":[{"description":"answer [Data: Reports (0)]","score":5}]}`, nil
}

func TestMapperReturnsRejectedResultAndReasonForCorrection(t *testing.T) {
	model := &correctingMapModel{}
	mapper, err := NewMapper(model, "{context_data} {max_length}", DefaultMapConfig())
	if err != nil {
		t.Fatalf("NewMapper() error = %v", err)
	}
	result, err := mapper.Map(t.Context(), MapRequest{
		Question: "question",
		Context: Context{Chunks: []ContextChunk{{
			Index: 0, Text: "report", ReportIDs: []int{0},
		}}},
	})
	if err != nil || result.Batches[0].Status != MapBatchSucceeded {
		t.Fatalf("Map() result/error = %#v/%v", result, err)
	}
	if model.correction.Result != "not-json" || !strings.Contains(model.correction.Reason, "JSON object") {
		t.Fatalf("correction = %#v", model.correction)
	}
}

func TestParseMapResponseRejectsMalformedResultParts(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		response string
		reason   string
	}{
		{
			name:     "wrapped JSON",
			response: "result: {\"points\":[]}",
			reason:   "decode Global model response",
		},
		{
			name:     "missing points",
			response: `{}`,
			reason:   "missing points",
		},
		{
			name:     "points is not an array",
			response: `{"points":{}}`,
			reason:   "decode Global Map points",
		},
		{
			name:     "point is not an object",
			response: `{"points":["bad"]}`,
			reason:   "point 0",
		},
		{
			name:     "point misses score",
			response: `{"points":[{"description":"answer"}]}`,
			reason:   "missing score",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseMapResponse(testCase.response)
			if err == nil || !strings.Contains(err.Error(), testCase.reason) {
				t.Fatalf("parseMapResponse() error = %v, want %q", err, testCase.reason)
			}
		})
	}
}

func (m citationMapModel) GenerateMap(context.Context, MapModelRequest) (string, error) {
	return m.response, nil
}

func TestMapperAcceptsOnlyReportsFromTheProducingChunk(t *testing.T) {
	contextData := Context{
		ReportSetID: "report-set",
		Chunks:      []ContextChunk{{Index: 0, Text: "batch", ReportIDs: []int{0, 2}}},
	}
	mapResponse := func(t *testing.T, response string) MapBatchResult {
		t.Helper()
		mapper, err := NewMapper(
			citationMapModel{response: response},
			"{context_data}",
			MapConfig{MaxLength: 1000, MaxConcurrency: 1},
		)
		if err != nil {
			t.Fatalf("NewMapper() error = %v", err)
		}
		result, err := mapper.Map(t.Context(), MapRequest{Question: "question", Context: contextData})
		if err != nil {
			t.Fatalf("Map() error = %v", err)
		}
		return result.Batches[0]
	}

	valid := mapResponse(
		t,
		`{"points":[{"description":"supported [Data: Reports (2, 0, 2)]","score":9}]}`,
	)
	if valid.Status != MapBatchSucceeded || len(valid.Points) != 1 ||
		!reflect.DeepEqual(valid.Points[0].ReportIDs, []int{2, 0}) {
		t.Fatalf("valid batch = %#v", valid)
	}

	for _, response := range []string{
		`{"points":[{"description":"other chunk [Data: Reports (1)]","score":9}]}`,
		`{"points":[{"description":"uncited","score":9}]}`,
		`{"points":[{"description":"wrong table [Data: Sources (0)]","score":9}]}`,
	} {
		batch := mapResponse(t, response)
		if batch.Status != MapBatchInvalidResponse || batch.Failure == nil ||
			batch.Failure.Category != querybase.FailureInvalidModelResponse ||
			len(batch.Points) != 1 || batch.Points[0].Score != 0 {
			t.Fatalf("invalid batch = %#v", batch)
		}
	}
}

type cancellingMapModel struct {
	started chan struct{}
}

func (m cancellingMapModel) GenerateMap(
	ctx context.Context,
	_ MapModelRequest,
) (string, error) {
	close(m.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func TestMapperPropagatesCancellation(t *testing.T) {
	started := make(chan struct{})
	mapper, err := NewMapper(
		cancellingMapModel{started: started},
		"{context_data}",
		MapConfig{MaxLength: 1, MaxConcurrency: 1},
	)
	if err != nil {
		t.Fatalf("NewMapper() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, mapErr := mapper.Map(ctx, MapRequest{Context: Context{
			Chunks: []ContextChunk{{Text: "batch"}},
		}})
		done <- mapErr
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Map() error = %v, want context.Canceled", err)
	}
}

func TestMapConfigurationAndStableRanking(t *testing.T) {
	for _, config := range []MapConfig{
		{MaxLength: 0, MaxConcurrency: 1},
		{MaxLength: 1, MaxConcurrency: 0},
	} {
		if _, err := NewMapper(&concurrentMapModel{}, "prompt", config); err == nil {
			t.Fatalf("NewMapper(%+v) error = nil", config)
		}
	}
	result := MapResult{Batches: []MapBatchResult{
		{Points: []MapPoint{{Description: "first tie", Score: 5, BatchIndex: 99, PointIndex: 99}}},
		{Points: []MapPoint{
			{Description: "highest", Score: 9, BatchIndex: 99, PointIndex: 99},
			{Description: "second tie", Score: 5, BatchIndex: 99, PointIndex: 99},
		}},
	}}
	points := result.RankedPoints()
	if len(points) != 3 ||
		points[0].Description != "highest" || points[0].BatchIndex != 1 || points[0].PointIndex != 0 ||
		points[1].Description != "first tie" || points[1].BatchIndex != 0 || points[1].PointIndex != 0 ||
		points[2].Description != "second tie" || points[2].BatchIndex != 1 || points[2].PointIndex != 1 {
		t.Fatalf("ranked points = %#v", points)
	}
}
