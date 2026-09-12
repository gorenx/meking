package question

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	querybase "github.com/memoria-space/meking/query"
)

type evidenceStub struct {
	evidence Evidence
	err      error
	requests []EvidenceRequest
}

func (s *evidenceStub) Prepare(_ context.Context, request EvidenceRequest) (Evidence, error) {
	s.requests = append(s.requests, request)
	return s.evidence, s.err
}

type modelStub struct {
	response string
	err      error
	requests []ModelRequest
}

type correctingModelStub struct {
	modelStub
	corrected  string
	correction CandidateCorrection
}

func (model *correctingModelStub) CorrectCandidates(
	_ context.Context,
	correction CandidateCorrection,
) (string, error) {
	model.correction = correction
	return model.corrected, nil
}

func (s *modelStub) Generate(_ context.Context, request ModelRequest) (string, error) {
	s.requests = append(s.requests, request)
	return s.response, s.err
}

type tokenCounterStub struct {
	count     int
	err       error
	corporaID string
	text      string
}

func (s *tokenCounterStub) Count(_ context.Context, corporaID, text string) (int, error) {
	s.corporaID, s.text = corporaID, text
	return s.count, s.err
}

func TestGeneratorUsesLastQuestionAndReturnsStableCandidates(t *testing.T) {
	evidence := &evidenceStub{evidence: Evidence{
		EpochID: 7, ReportSetID: "reports-1", CommunitySetID: "communities-1",
		CorporaID: "corpus-1", Context: "Entities\n0|A",
	}}
	model := &modelStub{response: " - First? \n\n- Second?\n- First?\nThird?\nFourth?"}
	tokens := &tokenCounterStub{count: 42}
	generator, err := NewGenerator(evidence, model, tokens, "count={question_count}\ncontext={context_data}")
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	result, err := generator.Suggest(t.Context(), Request{
		History: []string{"Earlier?", "What next?"}, Count: 3,
	})
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}
	if want := []string{"First?", "Second?", "Third?"}; !reflect.DeepEqual(result.Questions, want) {
		t.Fatalf("Questions = %#v, want %#v", result.Questions, want)
	}
	if result.Evidence.EpochID != 7 || result.PromptTokens != 42 || result.ModelCallCount != 1 {
		t.Fatalf("Result = %#v", result)
	}
	if want := []EvidenceRequest{{
		CurrentQuestion: "What next?", PreviousQuestions: []string{"Earlier?"},
	}}; !reflect.DeepEqual(evidence.requests, want) {
		t.Fatalf("Evidence requests = %#v, want %#v", evidence.requests, want)
	}
	if want := []ModelRequest{{
		SystemPrompt: "count=3\ncontext=Entities\n0|A", UserPrompt: "What next?",
	}}; !reflect.DeepEqual(model.requests, want) {
		t.Fatalf("Model requests = %#v, want %#v", model.requests, want)
	}
	if tokens.corporaID != "corpus-1" || tokens.text != "count=3\ncontext=Entities\n0|A" {
		t.Fatalf("Token input = %q/%q", tokens.corporaID, tokens.text)
	}
}

func TestGeneratorRejectsInputBeforeEvidence(t *testing.T) {
	evidence := &evidenceStub{}
	generator, err := NewGenerator(
		evidence, &modelStub{}, &tokenCounterStub{}, "{context_data}{question_count}",
	)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	invalid := []Request{
		{Count: 1},
		{History: []string{"question"}, Count: 0},
		{History: []string{"question"}, Count: MaximumCount + 1},
		{History: []string{"question", " "}, Count: 1},
	}
	for _, request := range invalid {
		_, err := generator.Suggest(t.Context(), request)
		var failure *querybase.Failure
		if !errors.As(err, &failure) || failure.Category != querybase.FailureInvalidInput {
			t.Fatalf("Suggest(%#v) error = %#v", request, err)
		}
	}
	if len(evidence.requests) != 0 {
		t.Fatalf("Evidence requests = %#v", evidence.requests)
	}
}

func TestGeneratorReturnsTypedFailures(t *testing.T) {
	evidence := &evidenceStub{evidence: Evidence{CorporaID: "corpus-1", Context: "context"}}
	model := &modelStub{response: " \n- \n"}
	generator, err := NewGenerator(
		evidence, model, &tokenCounterStub{}, "{context_data}{question_count}",
	)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	_, err = generator.Suggest(t.Context(), Request{History: []string{"question"}, Count: 1})
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != querybase.FailureInvalidModelResponse {
		t.Fatalf("Suggest(empty response) error = %#v", err)
	}

	evidence.err = querybase.NewNoEvidenceFailure(errors.New("none"))
	_, err = generator.Suggest(t.Context(), Request{History: []string{"question"}, Count: 1})
	if !errors.As(err, &failure) || failure.Category != querybase.FailureNoEvidence {
		t.Fatalf("Suggest(no evidence) error = %#v", err)
	}
}

func TestGeneratorReturnsRejectedCandidatesAndReasonForCorrection(t *testing.T) {
	evidence := &evidenceStub{evidence: Evidence{CorporaID: "corpus-1", Context: "context"}}
	model := &correctingModelStub{
		modelStub: modelStub{response: " \n- \n"},
		corrected: "- First?",
	}
	generator, err := NewGenerator(
		evidence, model, &tokenCounterStub{}, "{context_data}{question_count}",
	)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	result, err := generator.Suggest(t.Context(), Request{History: []string{"question"}, Count: 1})
	if err != nil || !reflect.DeepEqual(result.Questions, []string{"First?"}) || result.ModelCallCount != 2 {
		t.Fatalf("Suggest() result/error = %#v/%v", result, err)
	}
	if model.correction.Result != " \n- \n" || !strings.Contains(model.correction.Reason, "no non-empty candidates") {
		t.Fatalf("correction = %#v", model.correction)
	}
}

func TestNewGeneratorRequiresItsCompleteContract(t *testing.T) {
	evidence, model, tokens := &evidenceStub{}, &modelStub{}, &tokenCounterStub{}
	for _, test := range []struct {
		name     string
		evidence EvidenceProvider
		model    Model
		tokens   querybase.TokenCounter
		prompt   string
	}{
		{name: "evidence", model: model, tokens: tokens, prompt: "{context_data}{question_count}"},
		{name: "model", evidence: evidence, tokens: tokens, prompt: "{context_data}{question_count}"},
		{name: "tokens", evidence: evidence, model: model, prompt: "{context_data}{question_count}"},
		{name: "context field", evidence: evidence, model: model, tokens: tokens, prompt: "{question_count}"},
		{name: "count field", evidence: evidence, model: model, tokens: tokens, prompt: "{context_data}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewGenerator(test.evidence, test.model, test.tokens, test.prompt); err == nil {
				t.Fatal("NewGenerator() error = nil")
			}
		})
	}
}
