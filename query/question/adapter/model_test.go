package adapter

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/memoria-space/meking/agent"
	queryquestion "github.com/memoria-space/meking/query/question"
)

type completionStub struct {
	response           agent.CompletionStreamResponse
	completionResponse agent.CompletionResponse
	err                error
	requests           []agent.CompletionRequest
}

func (s *completionStub) Complete(
	_ context.Context,
	request agent.CompletionRequest,
) (agent.CompletionResponse, error) {
	s.requests = append(s.requests, request)
	return s.completionResponse, s.err
}

func TestCompletionModelReturnsRejectedCandidatesAndReasonToAgent(t *testing.T) {
	completion := &completionStub{completionResponse: agent.CompletionResponse{Content: "- Fixed?"}}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}
	response, err := model.CorrectCandidates(t.Context(), queryquestion.CandidateCorrection{
		Request: queryquestion.ModelRequest{SystemPrompt: "system", UserPrompt: "question"},
		Result:  "- ",
		Reason:  "no non-empty candidates",
	})
	if err != nil || response != "- Fixed?" {
		t.Fatalf("CorrectCandidates() response/error = %q/%v", response, err)
	}
	messages := completion.requests[0].Messages
	if len(messages) != 4 || messages[2].Content != "- " ||
		!strings.Contains(messages[3].Content, "no non-empty candidates") {
		t.Fatalf("correction messages = %#v", messages)
	}
}

func (s *completionStub) Stream(
	_ context.Context,
	request agent.CompletionRequest,
	emit agent.CompletionDeltaHandler,
) (agent.CompletionStreamResponse, error) {
	s.requests = append(s.requests, request)
	if s.response.Content != "" {
		_ = emit(s.response.Content)
	}
	return s.response, s.err
}

func TestCompletionModelUsesTwoMessageInternalStream(t *testing.T) {
	completion := &completionStub{response: agent.CompletionStreamResponse{Content: "- One?"}}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}
	response, err := model.Generate(t.Context(), queryquestion.ModelRequest{
		SystemPrompt: "system", UserPrompt: "current",
	})
	if err != nil || response != "- One?" {
		t.Fatalf("Generate() response/error = %q/%v", response, err)
	}
	want := []agent.CompletionRequest{{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: "system"},
			{Role: agent.CompletionRoleUser, Content: "current"},
		},
		ResponseFormat: agent.CompletionResponseText,
	}}
	if !reflect.DeepEqual(completion.requests, want) {
		t.Fatalf("requests = %#v, want %#v", completion.requests, want)
	}
}
