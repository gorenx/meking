package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	provider "github.com/memoria-space/meking/agent"
	"github.com/memoria-space/meking/community"
)

type completionClientStub struct {
	requests []provider.CompletionRequest
	results  []provider.CompletionResponse
	err      error
}

func (client *completionClientStub) Complete(
	_ context.Context,
	request provider.CompletionRequest,
) (provider.CompletionResponse, error) {
	client.requests = append(client.requests, request)
	if client.err != nil {
		return provider.CompletionResponse{}, client.err
	}
	if len(client.results) == 0 {
		return provider.CompletionResponse{}, errors.New("unexpected completion call")
	}
	result := client.results[0]
	client.results = client.results[1:]
	return result, nil
}

func TestReportModelMapsAndDecodesAgentResults(t *testing.T) {
	client := &completionClientStub{results: []provider.CompletionResponse{
		{Content: `{"title":"Alpha","summary":"Overview","findings":[{"summary":"Finding","explanation":"Evidence"}],"rating":7.5,"rating_explanation":"Important"}`},
		{Content: `{"content":"Grounded fragment"}`},
	}}
	model, err := NewReportModel(client)
	if err != nil {
		t.Fatalf("NewReportModel() error = %v", err)
	}
	draft, err := model.GenerateCommunityReport(t.Context(), community.ReportModelRequest{Prompt: "report"})
	if err != nil {
		t.Fatalf("GenerateCommunityReport() error = %v", err)
	}
	if draft.Title != "Alpha" || draft.Rating != 7.5 || len(draft.Findings) != 1 {
		t.Fatalf("draft = %#v", draft)
	}
	fragment, err := model.GenerateReportFragment(t.Context(), community.ReportFragmentRequest{Prompt: "fragment"})
	if err != nil {
		t.Fatalf("GenerateReportFragment() error = %v", err)
	}
	if fragment.Content != "Grounded fragment" {
		t.Fatalf("fragment = %#v", fragment)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d", len(client.requests))
	}
	for _, request := range client.requests {
		if request.ResponseFormat != provider.CompletionResponseJSONSchema || request.JSONSchema == nil {
			t.Fatalf("request = %#v", request)
		}
	}
}

func TestReportModelRejectsMalformedAgentResult(t *testing.T) {
	tests := []struct {
		name    string
		content string
		match   string
	}{
		{name: "invalid JSON", content: `{`, match: "unexpected EOF"},
		{name: "unknown field", content: `{"title":"A","summary":"S","findings":[],"rating":1,"rating_explanation":"R","extra":true}`, match: "unknown field"},
		{name: "missing field", content: `{"title":"A","summary":"S","findings":[],"rating":1}`, match: "rating_explanation"},
		{name: "trailing value", content: `{"title":"A","summary":"S","findings":[],"rating":1,"rating_explanation":"R"} {}`, match: "trailing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &completionClientStub{results: []provider.CompletionResponse{
				{Content: test.content},
				{Content: test.content},
			}}
			model, err := NewReportModel(client)
			if err != nil {
				t.Fatalf("NewReportModel() error = %v", err)
			}
			_, err = model.GenerateCommunityReport(t.Context(), community.ReportModelRequest{Prompt: "report"})
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("GenerateCommunityReport() error = %v, want %q", err, test.match)
			}
		})
	}
}

func TestReportModelReturnsRejectedResultAndReasonForCorrection(t *testing.T) {
	rejected := `{"title":"A"}`
	client := &completionClientStub{results: []provider.CompletionResponse{
		{Content: rejected},
		{Content: `{"title":"Alpha","summary":"Overview","findings":[],"rating":7,"rating_explanation":"Important"}`},
	}}
	model, err := NewReportModel(client)
	if err != nil {
		t.Fatalf("NewReportModel() error = %v", err)
	}

	draft, err := model.GenerateCommunityReport(t.Context(), community.ReportModelRequest{Prompt: "report"})
	if err != nil {
		t.Fatalf("GenerateCommunityReport() error = %v", err)
	}
	if draft.Title != "Alpha" || len(client.requests) != 2 {
		t.Fatalf("draft/requests = %#v/%d", draft, len(client.requests))
	}
	messages := client.requests[1].Messages
	if len(messages) != 3 || messages[1].Role != provider.CompletionRoleAssistant ||
		messages[1].Content != rejected || messages[2].Role != provider.CompletionRoleUser {
		t.Fatalf("correction messages = %#v", messages)
	}
	if !strings.Contains(messages[2].Content, rejected) ||
		!strings.Contains(messages[2].Content, "missing required field summary") {
		t.Fatalf("correction prompt = %q", messages[2].Content)
	}
}

func TestReportModelPropagatesAgentFailure(t *testing.T) {
	want := errors.New("agent unavailable")
	model, err := NewReportModel(&completionClientStub{err: want})
	if err != nil {
		t.Fatalf("NewReportModel() error = %v", err)
	}
	_, err = model.GenerateCommunityReport(t.Context(), community.ReportModelRequest{Prompt: "report"})
	if !errors.Is(err, want) {
		t.Fatalf("GenerateCommunityReport() error = %v", err)
	}
}
