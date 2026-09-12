package extraction

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGraphExtractorRunsGleaningConversationInBaselineOrder(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"first"}],"relations":[]}`},
		{Content: `{"entities":[{"name":"B","type":"thing","aliases":[],"description":"second"}],"relations":[]}`},
		{Content: "Y"},
		{Content: `{"entities":[],"relations":[{"source":"A","target":"B","type":"linked","description":"linked","weight":3}]}`},
	}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{
		Prompt:       "types={entity_types}; text={input_text}; object={{value}}",
		EntityTypes:  []string{"person", "geo"},
		MaxGleanings: 2,
	})

	got := extractOne(t, extractor, TextUnitInput{ID: "tu", Text: "  source text  "})
	if len(got.Entities) != 2 || len(got.Relationships) != 1 {
		t.Fatalf("graph observations = %#v, want two entities and one relationship", got)
	}
	if len(model.requests) != 4 {
		t.Fatalf("completion calls = %d, want 4", len(model.requests))
	}
	if gotPrompt := model.requests[0].Messages[0].Content; gotPrompt != "types=person,geo; text=source text; object={value}\n\n"+graphResultInstruction {
		t.Errorf("initial prompt = %q", gotPrompt)
	}
	wantRoles := [][]CompletionRole{
		{CompletionRoleUser},
		{CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser},
		{CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser},
		{CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser},
	}
	for index, request := range model.requests {
		roles := make([]CompletionRole, len(request.Messages))
		for messageIndex, message := range request.Messages {
			roles[messageIndex] = message.Role
		}
		if !reflect.DeepEqual(roles, wantRoles[index]) {
			t.Errorf("request[%d] roles = %v, want %v", index, roles, wantRoles[index])
		}
		wantsSchema := index != 2
		if got := len(request.Schema) != 0; got != wantsSchema {
			t.Errorf("request[%d] has schema = %v, want %v", index, got, wantsSchema)
		}
		if wantsSchema && request.SchemaName != graphExtractionSchemaName {
			t.Errorf("request[%d] schema name = %q", index, request.SchemaName)
		}
	}
	if got := model.requests[1].Messages[2].Content; got != continueExtractionPrompt {
		t.Errorf("continue prompt = %q", got)
	}
	if got := model.requests[2].Messages[4].Content; got != loopExtractionPrompt {
		t.Errorf("loop prompt = %q", got)
	}
}

func TestGraphExtractorAcceptsWhitespaceAroundContinuationDecision(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"first"}],"relations":[]}`},
		{Content: `{"entities":[],"relations":[{"source":"A","target":"A","type":"self","description":"self","weight":1}]}`},
		{Content: "Y\n"},
		{Content: `{"entities":[{"name":"B","type":"thing","aliases":[],"description":"second"}],"relations":[]}`},
	}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{input_text}", MaxGleanings: 2})
	_ = extractOne(t, extractor, TextUnitInput{ID: "tu", Text: "text"})
	if len(model.requests) != 4 {
		t.Fatalf("completion calls = %d, want whitespace-trimmed Y to continue", len(model.requests))
	}
}

func TestGraphExtractorRejectsInvalidContinuationDecision(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"first"}],"relations":[]}`},
		{Content: `{"entities":[{"name":"B","type":"thing","aliases":[],"description":"second"}],"relations":[]}`},
		{Content: "maybe"},
		{Content: "still maybe"},
	}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{
		Prompt:       "{input_text}",
		MaxGleanings: 2,
	})

	_, err := extractor.processDocument(t.Context(), "text")
	if !errors.Is(err, ErrInvalidGraphExtraction) {
		t.Fatalf("processDocument() error = %v, want ErrInvalidGraphExtraction", err)
	}
}

func TestGraphExtractorReturnsInvalidContinuationDecisionForCorrection(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"first"}],"relations":[]}`},
		{Content: `{"entities":[{"name":"B","type":"thing","aliases":[],"description":"second"}],"relations":[]}`},
		{Content: "maybe"},
		{Content: "N"},
	}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{
		Prompt: "{input_text}", MaxGleanings: 2,
	})
	conversation, err := extractor.processDocument(t.Context(), "text")
	if err != nil {
		t.Fatalf("processDocument() error = %v", err)
	}
	if conversation.result == "" || len(model.requests) != 4 {
		t.Fatalf("conversation/calls = %#v/%d", conversation, len(model.requests))
	}
	feedback := model.requests[3].Messages[len(model.requests[3].Messages)-1]
	if !strings.Contains(feedback.Content, "maybe") ||
		!strings.Contains(feedback.Content, "must be Y or N") {
		t.Fatalf("correction feedback = %#v", feedback)
	}
}

func TestGraphExtractorReturnsRejectedRecordAndReasonForCorrection(t *testing.T) {
	invalid := `{"entities":[],"relations":[{"source":"A","target":"B","type":"","description":"linked","weight":1}]}`
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: invalid},
		{Content: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"first"},{"name":"B","type":"thing","aliases":[],"description":"second"}],"relations":[{"source":"A","target":"B","type":"linked","description":"linked","weight":1}]}`},
	}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{input_text}"})

	observations, err := extractor.extractGraph(t.Context(), []TextUnitInput{{ID: "tu", Text: "text"}})
	if err != nil {
		t.Fatalf("extractGraph() error = %v", err)
	}
	if len(observations.Entities) != 2 || len(observations.Relationships) != 1 {
		t.Fatalf("extractGraph() = %#v", observations)
	}
	if len(model.requests) != 2 {
		t.Fatalf("completion calls = %d, want initial call and one correction", len(model.requests))
	}
	correction := model.requests[1].Messages
	if len(correction) != 3 || correction[1].Role != CompletionRoleAssistant ||
		correction[1].Content != invalid || correction[2].Role != CompletionRoleUser {
		t.Fatalf("correction conversation = %#v", correction)
	}
	if !strings.Contains(correction[2].Content, `"type":""`) ||
		!strings.Contains(correction[2].Content, "source, target, type, description") {
		t.Fatalf("correction prompt = %q", correction[2].Content)
	}
	if len(model.requests[1].Schema) == 0 {
		t.Fatal("correction request has no graph schema")
	}
}

func TestGraphExtractorRejectsResultAfterOneFailedCorrection(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: `{"entities":[],"relations":[{"source":"A","target":"B","type":"","description":"linked","weight":1}]}`},
		{Content: `{"entities":[],"relations":[{"source":"A","target":"B","type":"","description":"still invalid","weight":1}]}`},
	}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{input_text}"})

	_, err := extractor.extractGraph(t.Context(), []TextUnitInput{{ID: "tu", Text: "text"}})
	if !errors.Is(err, ErrInvalidGraphExtraction) {
		t.Fatalf("extractGraph() error = %v, want invalid extraction", err)
	}
	if len(model.requests) != 2 {
		t.Fatalf("completion calls = %d, want correction bounded to one", len(model.requests))
	}
}

func TestGraphExtractorReportsAndRejectsPerUnitFailure(t *testing.T) {
	expected := errors.New("provider failed")
	model := &scriptedCompletionModel{errors: []error{expected}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{input_text}"})

	_, err := extractor.extractGraph(context.Background(), []TextUnitInput{{ID: "tu-1", Text: "text"}})
	if !errors.Is(err, expected) {
		t.Fatalf("extractGraph() error = %v, want provider failure", err)
	}
	if !strings.Contains(err.Error(), `"tu-1"`) {
		t.Fatalf("extractGraph() error = %v, want TextUnit identity", err)
	}
}

func TestGraphExtractorRejectsIncompleteAuthoritativeBatch(t *testing.T) {
	firstFailure := errors.New("first provider failure")
	secondFailure := errors.New("second provider failure")
	model := &scriptedCompletionModel{
		errors: []error{firstFailure, secondFailure},
	}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{
		Prompt: "{input_text}", MaxConcurrency: 1,
	})

	_, err := extractor.extractGraph(context.Background(), []TextUnitInput{
		{ID: "tu-1", Text: "first"},
		{ID: "tu-2", Text: "second"},
	})
	if err == nil {
		t.Fatal("extractGraph() error = nil")
	}
	firstIndex := strings.Index(err.Error(), `"tu-1"`)
	secondIndex := strings.Index(err.Error(), `"tu-2"`)
	if !errors.Is(err, firstFailure) || !errors.Is(err, secondFailure) ||
		firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("extractGraph() error = %v, want both failures in TextUnit order", err)
	}
}

func TestGraphExtractorPropagatesCancellation(t *testing.T) {
	model := &scriptedCompletionModel{errors: []error{context.Canceled}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{input_text}"})
	_, err := extractor.extractGraph(context.Background(), []TextUnitInput{{ID: "tu", Text: "text"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("extractGraph() error = %v, want context.Canceled", err)
	}
}

func TestGraphExtractorRejectsOrphans(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"a"}],"relations":[{"source":"A","target":"B","type":"related","description":"valid later","weight":2},{"source":"A","target":"GHOST","type":"related","description":"orphan","weight":4}]}`},
		{Content: `{"entities":[{"name":"B","type":"thing","aliases":[],"description":"b"}],"relations":[]}`},
	}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{input_text}"})
	_, err := extractor.extractGraph(context.Background(), []TextUnitInput{
		{ID: "tu-1", Text: "first"},
		{ID: "tu-2", Text: "second"},
	})
	if !errors.Is(err, ErrUnresolvedEntityReference) {
		t.Fatalf("extractGraph() error = %v, want ErrUnresolvedEntityReference", err)
	}
}

func TestGraphExtractorRejectsEmptyStageResults(t *testing.T) {
	tests := []struct {
		name     string
		response string
		expected error
	}{
		{name: "entities", response: `{"entities":[],"relations":[]}`, expected: ErrNoEntitiesDetected},
		{name: "orphan relationships", response: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"a"}],"relations":[{"source":"A","target":"B","type":"related","description":"orphan","weight":1}]}`, expected: ErrUnresolvedEntityReference},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &scriptedCompletionModel{responses: []CompletionResponse{{Content: test.response}}}
			extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{input_text}"})
			_, err := extractor.extractGraph(context.Background(), []TextUnitInput{{ID: "tu", Text: "text"}})
			if !errors.Is(err, test.expected) {
				t.Fatalf("extractGraph() error = %v, want %v", err, test.expected)
			}
		})
	}
}

func TestGraphExtractorReportsPromptFormattingFailure(t *testing.T) {
	model := &scriptedCompletionModel{}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{unknown}"})
	_, err := extractor.extractGraph(context.Background(), []TextUnitInput{{ID: "tu", Text: "text"}})
	if err == nil || !strings.Contains(err.Error(), `unknown prompt field "unknown"`) {
		t.Fatalf("extractGraph() error = %v, want Prompt formatting failure", err)
	}
	if len(model.requests) != 0 {
		t.Fatalf("format failure called model: requests=%d", len(model.requests))
	}
	if !strings.Contains(err.Error(), `TextUnit "tu"`) {
		t.Fatalf("extractGraph() error = %v, want TextUnit identity", err)
	}
}

func TestNewGraphExtractorValidatesDependencyAndCopiesConfig(t *testing.T) {
	if _, err := newGraphExtractor(nil, GraphExtractionConfig{}); err == nil {
		t.Fatal("newGraphExtractor(nil) error = nil")
	}
	if _, err := newGraphExtractor(&scriptedCompletionModel{}, GraphExtractionConfig{MaxConcurrency: -1}); err == nil {
		t.Fatal("newGraphExtractor(negative concurrency) error = nil")
	}
	types := []string{"person"}
	model := &scriptedCompletionModel{responses: []CompletionResponse{{Content: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"a"}],"relations":[]}`}}}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{Prompt: "{entity_types}", EntityTypes: types})
	types[0] = "changed"
	_ = extractOne(t, extractor, TextUnitInput{ID: "tu"})
	if got := model.requests[0].Messages[0].Content; got != "person\n\n"+graphResultInstruction {
		t.Fatalf("rendered entity types = %q, want copied config", got)
	}
}

func TestGraphExtractorBoundsConcurrencyAndRestoresInputOrder(t *testing.T) {
	model := &boundedExtractionModel{
		started: make(chan struct{}, 3),
		release: make(chan struct{}),
	}
	extractor := mustgraphExtractor(t, model, GraphExtractionConfig{
		Prompt: "{input_text}", MaxConcurrency: 2,
	})
	results := make(chan graphObservations, 1)
	errors := make(chan error, 1)
	go func() {
		observations, err := extractor.extractGraph(context.Background(), []TextUnitInput{
			{ID: "tu-1", Text: "one"}, {ID: "tu-2", Text: "two"}, {ID: "tu-3", Text: "three"},
		})
		results <- observations
		errors <- err
	}()

	waitForExtractionStarts(t, model.started, 2)
	select {
	case <-model.started:
		t.Fatal("third extraction started above MaxConcurrency")
	case <-time.After(30 * time.Millisecond):
	}
	close(model.release)
	if err := <-errors; err != nil {
		t.Fatalf("extractGraph() error = %v", err)
	}
	observations := <-results
	if model.maximum.Load() != 2 {
		t.Fatalf("maximum concurrent calls = %d, want 2", model.maximum.Load())
	}
	gotIDs := make([]string, len(observations.Entities))
	for index, entity := range observations.Entities {
		gotIDs[index] = entity.TextUnitID
	}
	wantIDs := []string{"tu-1", "tu-1", "tu-2", "tu-2", "tu-3", "tu-3"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("entity source order = %v, want %v", gotIDs, wantIDs)
	}
}

type boundedExtractionModel struct {
	started chan struct{}
	release chan struct{}
	current atomic.Int64
	maximum atomic.Int64
}

func (m *boundedExtractionModel) Complete(context.Context, CompletionRequest) (CompletionResponse, error) {
	current := m.current.Add(1)
	for maximum := m.maximum.Load(); current > maximum && !m.maximum.CompareAndSwap(maximum, current); maximum = m.maximum.Load() {
	}
	m.started <- struct{}{}
	<-m.release
	m.current.Add(-1)
	return CompletionResponse{Content: `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"a"},{"name":"B","type":"thing","aliases":[],"description":"b"}],"relations":[{"source":"A","target":"B","type":"linked","description":"linked","weight":1}]}`}, nil
}

func waitForExtractionStarts(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for concurrent call %d", index+1)
		}
	}
}

func mustgraphExtractor(t *testing.T, model CompletionModel, config GraphExtractionConfig) *graphExtractor {
	t.Helper()
	extractor, err := newGraphExtractor(model, config)
	if err != nil {
		t.Fatalf("newGraphExtractor() error = %v", err)
	}
	return extractor
}

func extractOne(t *testing.T, extractor *graphExtractor, unit TextUnitInput) graphObservations {
	t.Helper()
	conversation, err := extractor.processDocument(t.Context(), strings.TrimSpace(unit.Text))
	if err != nil {
		t.Fatalf("processDocument() error = %v", err)
	}
	observations, err := parseGraphExtraction(conversation.result, unit.ID)
	if err != nil {
		t.Fatalf("parseGraphExtraction() error = %v", err)
	}
	return observations
}

type scriptedCompletionModel struct {
	responses []CompletionResponse
	errors    []error
	requests  []CompletionRequest
}

func (m *scriptedCompletionModel) Complete(_ context.Context, request CompletionRequest) (CompletionResponse, error) {
	request.Messages = append([]CompletionMessage(nil), request.Messages...)
	m.requests = append(m.requests, request)
	index := len(m.requests) - 1
	if index < len(m.errors) && m.errors[index] != nil {
		return CompletionResponse{}, m.errors[index]
	}
	if index >= len(m.responses) {
		return CompletionResponse{}, errors.New("unexpected completion call")
	}
	return m.responses[index], nil
}
