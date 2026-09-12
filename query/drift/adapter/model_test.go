package adapter

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/memoria-space/meking/agent"
	querydrift "github.com/memoria-space/meking/query/drift"
)

type driftCompletion struct {
	requests []agent.CompletionRequest
	content  string
}

func (s *driftCompletion) Complete(
	_ context.Context,
	request agent.CompletionRequest,
) (agent.CompletionResponse, error) {
	s.requests = append(s.requests, request)
	return agent.CompletionResponse{Content: s.content}, nil
}

func TestCompletionModelMapsDistinctDRIFTOperations(t *testing.T) {
	completion := &driftCompletion{content: "response"}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}
	request := querydrift.ModelRequest{Prompt: "prompt", MaxCompletionTokens: 321}
	if response, err := model.GenerateHypothetical(t.Context(), request); err != nil || response != "response" {
		t.Fatalf("GenerateHypothetical() = %q, %v", response, err)
	}
	if response, err := model.GeneratePrimer(t.Context(), request); err != nil || response != "response" {
		t.Fatalf("GeneratePrimer() = %q, %v", response, err)
	}
	if response, err := model.GenerateBranch(t.Context(), querydrift.BranchModelRequest{
		SystemPrompt: "system", UserPrompt: "question", MaxCompletionTokens: 123,
	}); err != nil || response != "response" {
		t.Fatalf("GenerateBranch() = %q, %v", response, err)
	}
	if response, err := model.GenerateReduce(t.Context(), querydrift.ReduceModelRequest{
		SystemPrompt: "reduce system", UserPrompt: "question", MaxCompletionTokens: 456,
	}); err != nil || response != "response" {
		t.Fatalf("GenerateReduce() = %q, %v", response, err)
	}
	if len(completion.requests) != 4 {
		t.Fatalf("requests = %#v", completion.requests)
	}
	for _, got := range completion.requests[:2] {
		if !reflect.DeepEqual(got.Messages, []agent.CompletionMessage{{
			Role: agent.CompletionRoleUser, Content: "prompt",
		}}) || got.MaxCompletionTokens != 321 {
			t.Fatalf("request = %#v", got)
		}
	}
	if completion.requests[0].ResponseFormat != agent.CompletionResponseText ||
		completion.requests[1].ResponseFormat != agent.CompletionResponseJSONObject {
		t.Fatalf("response formats = %q/%q", completion.requests[0].ResponseFormat, completion.requests[1].ResponseFormat)
	}
	wantBranch := agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: "system"},
			{Role: agent.CompletionRoleUser, Content: "question"},
		},
		ResponseFormat:      agent.CompletionResponseJSONObject,
		MaxCompletionTokens: 123,
	}
	if !reflect.DeepEqual(completion.requests[2], wantBranch) {
		t.Fatalf("branch request = %#v, want %#v", completion.requests[2], wantBranch)
	}
	wantReduce := agent.CompletionRequest{
		Messages: []agent.CompletionMessage{
			{Role: agent.CompletionRoleSystem, Content: "reduce system"},
			{Role: agent.CompletionRoleUser, Content: "question"},
		},
		ResponseFormat:      agent.CompletionResponseText,
		MaxCompletionTokens: 456,
	}
	if !reflect.DeepEqual(completion.requests[3], wantReduce) {
		t.Fatalf("Reduce request = %#v, want %#v", completion.requests[3], wantReduce)
	}
}

func TestCompletionModelReturnsRejectedDRIFTResultAndReasonToAgent(t *testing.T) {
	completion := &driftCompletion{content: `{"intermediate_answer":"fixed","score":5,"follow_up_queries":["next"]}`}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}
	response, err := model.CorrectPrimer(t.Context(), querydrift.PrimerCorrection{
		Request: querydrift.ModelRequest{Prompt: "prompt", MaxCompletionTokens: 123},
		Result:  `{"score":101}`,
		Reason:  "score must be between 0 and 100",
	})
	if err != nil || response == "" {
		t.Fatalf("CorrectPrimer() response/error = %q/%v", response, err)
	}
	request := completion.requests[0]
	if len(request.Messages) != 3 || request.Messages[1].Role != agent.CompletionRoleAssistant ||
		request.Messages[1].Content != `{"score":101}` ||
		!strings.Contains(request.Messages[2].Content, "score must be between 0 and 100") ||
		request.ResponseFormat != agent.CompletionResponseJSONObject || request.MaxCompletionTokens != 123 {
		t.Fatalf("correction request = %#v", request)
	}
}

type streamingDriftCompletion struct {
	driftCompletion
}

func (s *streamingDriftCompletion) Stream(
	_ context.Context,
	request agent.CompletionRequest,
	emit agent.CompletionDeltaHandler,
) (agent.CompletionStreamResponse, error) {
	s.requests = append(s.requests, request)
	if err := emit("streamed"); err != nil {
		return agent.CompletionStreamResponse{Content: "streamed"}, err
	}
	return agent.CompletionStreamResponse{Content: "streamed"}, nil
}

func TestCompletionModelStreamsOnlyReduceText(t *testing.T) {
	completion := &streamingDriftCompletion{}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}
	var delta string
	response, err := model.StreamReduce(t.Context(), querydrift.ReduceModelRequest{
		SystemPrompt: "system", UserPrompt: "question", MaxCompletionTokens: 99,
	}, func(value string) error {
		delta += value
		return nil
	})
	if err != nil || response != "streamed" || delta != "streamed" ||
		len(completion.requests) != 1 || completion.requests[0].MaxCompletionTokens != 99 {
		t.Fatalf("StreamReduce() = %q, %v, delta %q, requests %#v", response, err, delta, completion.requests)
	}
}
