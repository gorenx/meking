package agent

import (
	"context"
	"encoding/json"
	"testing"

	provider "github.com/memoria-space/meking/agent"
	"github.com/memoria-space/meking/knowledge/extraction"
)

type recordingModel struct {
	request provider.CompletionRequest
}

func (model *recordingModel) Complete(
	_ context.Context,
	request provider.CompletionRequest,
) (provider.CompletionResponse, error) {
	model.request = request
	return provider.CompletionResponse{Content: `{"entities":[],"relations":[]}`}, nil
}

func TestCompletionMapsExtractionSchemaToAgentRequest(t *testing.T) {
	model := &recordingModel{}
	completion, err := NewCompletion(model)
	if err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"type":"object"}`)
	_, err = completion.Complete(t.Context(), extraction.CompletionRequest{
		Messages: []extraction.CompletionMessage{{
			Role:    extraction.CompletionRoleUser,
			Content: "extract",
		}},
		SchemaName: "knowledge_graph",
		Schema:     schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.request.ResponseFormat != provider.CompletionResponseJSONSchema ||
		model.request.JSONSchema == nil ||
		model.request.JSONSchema.Name != "knowledge_graph" ||
		string(model.request.JSONSchema.Definition) != string(schema) {
		t.Fatalf("Agent request = %#v", model.request)
	}
}

func TestCompletionLeavesUnstructuredExtractionRequestAsText(t *testing.T) {
	model := &recordingModel{}
	completion, err := NewCompletion(model)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := completion.Complete(t.Context(), extraction.CompletionRequest{}); err != nil {
		t.Fatal(err)
	}
	if model.request.ResponseFormat != provider.CompletionResponseText || model.request.JSONSchema != nil {
		t.Fatalf("Agent request = %#v", model.request)
	}
}
