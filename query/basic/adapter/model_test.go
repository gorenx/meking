package adapter_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/memoria-space/meking/agent"
	querybase "github.com/memoria-space/meking/query"
	querybasic "github.com/memoria-space/meking/query/basic"
	querybasicadapter "github.com/memoria-space/meking/query/basic/adapter"
)

func TestCompletionModelMapsBasicMessagesAndStreaming(t *testing.T) {
	completion := &completionStub{response: "answer", streamResponse: "streamed"}
	adapter, err := querybasicadapter.NewCompletionModel(completion)
	if err != nil {
		t.Fatalf("NewCompletionModel() error = %v", err)
	}
	request := querybasic.AnswerModelRequest{
		SystemPrompt: "system",
		UserPrompt:   "question",
	}
	answer, err := adapter.GenerateAnswer(t.Context(), request)
	if err != nil || answer != "answer" {
		t.Fatalf("GenerateAnswer() = %q, %v", answer, err)
	}
	wantMessages := []agent.CompletionMessage{
		{Role: agent.CompletionRoleSystem, Content: "system"},
		{Role: agent.CompletionRoleUser, Content: "question"},
	}
	if !reflect.DeepEqual(completion.request.Messages, wantMessages) ||
		completion.request.ResponseFormat != agent.CompletionResponseText {
		t.Fatalf("completion request = %#v", completion.request)
	}
	deltas := ""
	answer, err = adapter.StreamAnswer(t.Context(), request, func(delta string) error {
		deltas += delta
		return nil
	})
	if err != nil || answer != "streamed" || deltas != "streamed" {
		t.Fatalf("StreamAnswer() = %q/%q, %v", answer, deltas, err)
	}
}

func TestCompletionModelClassifiesModelFailure(t *testing.T) {
	want := errors.New("model failed")
	adapter, _ := querybasicadapter.NewCompletionModel(&completionStub{err: want})
	_, err := adapter.GenerateAnswer(t.Context(), querybasic.AnswerModelRequest{})
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != querybase.FailureInternal ||
		!errors.Is(err, want) {
		t.Fatalf("GenerateAnswer() error = %#v", err)
	}
}

type completionStub struct {
	request        agent.CompletionRequest
	response       string
	streamResponse string
	err            error
}

func (s *completionStub) Complete(
	_ context.Context,
	request agent.CompletionRequest,
) (agent.CompletionResponse, error) {
	s.request = request
	return agent.CompletionResponse{Content: s.response}, s.err
}

func (s *completionStub) Stream(
	_ context.Context,
	request agent.CompletionRequest,
	emit agent.CompletionDeltaHandler,
) (agent.CompletionStreamResponse, error) {
	s.request = request
	if s.streamResponse != "" {
		if err := emit(s.streamResponse); err != nil {
			return agent.CompletionStreamResponse{Content: s.streamResponse}, err
		}
	}
	return agent.CompletionStreamResponse{Content: s.streamResponse}, s.err
}
