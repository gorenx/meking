package adapter

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/memoria-space/meking/agent"
	querybase "github.com/memoria-space/meking/query"
	queryglobal "github.com/memoria-space/meking/query/global"
)

type completionStub struct {
	request  agent.CompletionRequest
	response agent.CompletionResponse
	err      error
}

func (s *completionStub) Complete(
	_ context.Context,
	request agent.CompletionRequest,
) (agent.CompletionResponse, error) {
	s.request = request
	return s.response, s.err
}

type streamCompletionStub struct {
	completionStub
	deltas []string
}

func (s *streamCompletionStub) Stream(
	_ context.Context,
	request agent.CompletionRequest,
	emit agent.CompletionDeltaHandler,
) (agent.CompletionStreamResponse, error) {
	s.request = request
	content := ""
	for _, delta := range s.deltas {
		if err := emit(delta); err != nil {
			return agent.CompletionStreamResponse{Content: content}, err
		}
		content += delta
	}
	return agent.CompletionStreamResponse{Content: content}, s.err
}

func TestCompletionModelMapsMapRatingAndReduceFormats(t *testing.T) {
	completion := &completionStub{response: agent.CompletionResponse{Content: "response"}}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}

	response, err := model.GenerateMap(t.Context(), queryglobal.MapModelRequest{
		SystemPrompt: "map system", UserPrompt: "question",
	})
	if err != nil || response != "response" ||
		completion.request.ResponseFormat != agent.CompletionResponseJSONObject {
		t.Fatalf("GenerateMap() response/request/error = %q/%#v/%v", response, completion.request, err)
	}
	wantMessages := []agent.CompletionMessage{
		{Role: agent.CompletionRoleSystem, Content: "map system"},
		{Role: agent.CompletionRoleUser, Content: "question"},
	}
	if !reflect.DeepEqual(completion.request.Messages, wantMessages) {
		t.Fatalf("Map messages = %#v", completion.request.Messages)
	}

	_, err = model.RateCommunity(t.Context(), queryglobal.RatingRequest{
		SystemPrompt: "rate system", UserPrompt: "question",
	})
	if err != nil || completion.request.ResponseFormat != agent.CompletionResponseJSONObject {
		t.Fatalf("RateCommunity() request/error = %#v/%v", completion.request, err)
	}
	_, err = model.GenerateReduce(t.Context(), queryglobal.ReduceModelRequest{
		SystemPrompt: "reduce system", UserPrompt: "question",
	})
	if err != nil || completion.request.ResponseFormat != agent.CompletionResponseText {
		t.Fatalf("GenerateReduce() request/error = %#v/%v", completion.request, err)
	}
}

func TestCompletionModelReturnsRejectedResultAndReasonToAgent(t *testing.T) {
	completion := &completionStub{response: agent.CompletionResponse{Content: `{"rating":3}`}}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}

	response, err := model.CorrectRating(t.Context(), queryglobal.RatingCorrection{
		Request: queryglobal.RatingRequest{SystemPrompt: "system", UserPrompt: "question"},
		Result:  "not-json",
		Reason:  "expected JSON object",
	})
	if err != nil || response != `{"rating":3}` {
		t.Fatalf("CorrectRating() response/error = %q/%v", response, err)
	}
	if len(completion.request.Messages) != 4 ||
		completion.request.Messages[2] != (agent.CompletionMessage{Role: agent.CompletionRoleAssistant, Content: "not-json"}) ||
		!strings.Contains(completion.request.Messages[3].Content, "expected JSON object") {
		t.Fatalf("correction request = %#v", completion.request)
	}
}

func TestCompletionModelStreamsFinalText(t *testing.T) {
	completion := &streamCompletionStub{deltas: []string{"final", " answer"}}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}
	var deltas []string
	response, err := model.StreamReduce(
		t.Context(),
		queryglobal.ReduceModelRequest{SystemPrompt: "system", UserPrompt: "question"},
		func(delta string) error {
			deltas = append(deltas, delta)
			return nil
		},
	)
	if err != nil || response != "final answer" ||
		!reflect.DeepEqual(deltas, completion.deltas) {
		t.Fatalf("StreamReduce() response/deltas/error = %q/%#v/%v", response, deltas, err)
	}
}

func TestCompletionModelClassifiesFailures(t *testing.T) {
	completion := &completionStub{err: context.Canceled}
	model, err := NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}
	_, err = model.GenerateReduce(t.Context(), queryglobal.ReduceModelRequest{})
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != querybase.FailureCancelled {
		t.Fatalf("GenerateReduce() error = %#v", err)
	}
}
