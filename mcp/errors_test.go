package mcp

import (
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/deletion"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/zone"
)

func TestClassifyStorageFailure(t *testing.T) {
	for _, err := range []error{corpus.ErrCorpusStorageBusy, knowledge.ErrStorageBusy} {
		failure := classifyFailure(err)
		if failure.Code != "storage_unavailable" || !failure.Retryable {
			t.Fatalf("failure for %v = %#v", err, failure)
		}
	}
}

func TestClassifyDomainFailureWithoutInspectingMessage(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{err: deletion.ErrKnowledgeInUse, code: "knowledge_in_use"},
		{err: resolution.ErrConflictNotFound, code: "conflict_not_found"},
		{err: zone.ErrInvalidUser, code: "invalid_user"},
	} {
		failure := classifyFailure(test.err)
		if failure.Code != test.code {
			t.Fatalf("failure for %v = %#v", test.err, failure)
		}
	}
}

func TestFailureResultUsesStructuredError(t *testing.T) {
	failure := ToolFailure{
		Code:    "invalid_memory",
		Message: "Knowledge is required",
	}
	result := failureResult(failure)
	if !result.IsError {
		t.Fatal("failure result is not marked as an error")
	}
	if result.StructuredContent != failure {
		t.Fatalf("failure StructuredContent = %#v", result.StructuredContent)
	}
	if len(result.Content) != 1 {
		t.Fatalf("failure Content = %#v", result.Content)
	}
}
