package assembly_test

import (
	"strings"
	"testing"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/memoria-space/meking/knowledge"
	mekingmcp "github.com/memoria-space/meking/mcp"
	"github.com/memoria-space/meking/memory"
)

func TestMCPRejectsInvalidMemoryBeforeWriting(t *testing.T) {
	fixture := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	sessionID := string(fixture.sessionID)

	unknownField := rawToolCall(t, fixture.client, "add_memory", map[string]any{
		"user_id":    mcpTestUserID,
		"session_id": sessionID,
		"memory": map[string]any{
			"id": "schema-memory",
			"messages": []any{map[string]any{
				"id": "schema-message", "role": "user", "text": "Alice is an engineer.",
			}},
			"knowledge": map[string]any{
				"entities": []any{}, "relations": []any{}, "claims": []any{},
			},
		},
		"unknown": true,
	})
	assertSchemaError(t, unknownField, "/unknown")
	callTool[mekingmcp.MemoryReceipt](
		t,
		fixture.client,
		"add_memory",
		memoryRequest(sessionID, "schema-memory", "schema-message", "Alice is an engineer."),
	)

	wrongType := rawToolCall(t, fixture.client, "add_memory", map[string]any{
		"user_id":    mcpTestUserID,
		"session_id": sessionID,
		"memory": map[string]any{
			"id":       "type-memory",
			"messages": "not-an-array",
			"knowledge": map[string]any{
				"entities": []any{}, "relations": []any{}, "claims": []any{},
			},
		},
	})
	assertSchemaError(t, wrongType, "/memory/messages")
	callTool[mekingmcp.MemoryReceipt](
		t,
		fixture.client,
		"add_memory",
		memoryRequest(sessionID, "type-memory", "type-message", "Alice is an engineer."),
	)

	invalidSubject := memoryRequest(
		sessionID,
		"subject-memory",
		"subject-message",
		"Alice is an engineer.",
	)
	invalidSubject.Memory.Knowledge.Claims = []mekingmcp.ClaimMemory{{
		Type:        "ROLE",
		Description: "Alice is an engineer.",
		Source:      mekingmcp.ClaimSource{SubjectText: "Alice", ObjectText: "engineer"},
	}}
	assertToolFailure(t, rawToolCall(t, fixture.client, "add_memory", invalidSubject), "invalid_memory")
	invalidSubject.Memory.Knowledge.Claims = []mekingmcp.ClaimMemory{}
	callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", invalidSubject)

	emptyDescription := memoryRequest(
		sessionID,
		"description-memory",
		"description-message",
		"Alice is an engineer.",
	)
	emptyDescription.Memory.Knowledge.Entities[0].Content.Description = ""
	assertToolFailure(t, rawToolCall(t, fixture.client, "add_memory", emptyDescription), "invalid_memory")
	emptyDescription.Memory.Knowledge.Entities[0].Content.Description = "Alice is an engineer."
	callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", emptyDescription)

	unknownReference := memoryRequest(
		sessionID,
		"reference-memory",
		"reference-message",
		"Alice knows Bob.",
	)
	unknownReference.Memory.Knowledge.Relations = []mekingmcp.RelationMemory{{
		Content: knowledge.RelationContent{
			Source:      knowledge.EntityIdentity{Title: "ALICE", Type: "PERSON"},
			Target:      knowledge.EntityIdentity{Title: "BOB", Type: "PERSON"},
			Type:        "KNOWS",
			Description: "Alice knows Bob.",
		},
		Source: mekingmcp.RelationSource{Weight: 1},
	}}
	assertToolFailure(t, rawToolCall(t, fixture.client, "add_memory", unknownReference), "invalid_memory")
	unknownReference.Memory.Knowledge.Relations = []mekingmcp.RelationMemory{}
	callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", unknownReference)
}

func TestMCPMemoryIdentityAndCanonicalContent(t *testing.T) {
	fixture := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	sessionID := string(fixture.sessionID)
	first := memoryRequest(sessionID, "memory-1", "message-1", "Alice is an engineer.")
	callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", first)

	reused := callTool[mekingmcp.MemoryReceipt](
		t,
		fixture.client,
		"add_memory",
		memoryRequest(sessionID, "memory-2", "message-1", "Alice is an engineer."),
	)
	if len(reused.Messages) != 1 || reused.Messages[0].Position != 0 {
		t.Fatalf("reused Message = %#v", reused.Messages)
	}
	assertToolFailure(
		t,
		rawToolCall(
			t,
			fixture.client,
			"add_memory",
			memoryRequest(sessionID, "memory-3", "message-1", "Alice is a designer."),
		),
		"message_identity_conflict",
	)

	confirmed := callTool[mekingmcp.MemoryReceipt](
		t,
		fixture.client,
		"add_memory",
		memoryRequest(sessionID, "memory-4", "message-2", "Alice is an engineer."),
	)
	if len(confirmed.Conflicts.Entities) != 0 {
		t.Fatalf("identical content conflicts = %#v", confirmed.Conflicts.Entities)
	}
	result := callTool[mekingmcp.MemoryMatches](t, fixture.client, "search_memory", mekingmcp.SearchMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: sessionID,
		Query: memory.Query{
			Title: "ALICE",
			Type:  "PERSON",
		},
	})
	if len(result.Entities) != 1 || result.Entities[0].Version.Number != 1 ||
		len(result.Entities[0].Evidence) != 3 || len(result.Messages) != 2 {
		t.Fatalf("confirmed Entity = %#v", result.Entities)
	}
}

func TestMCPMessagePositionsAndTextUnitIdentity(t *testing.T) {
	fixture := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	sessionID := string(fixture.sessionID)
	request := memoryRequest(sessionID, "memory-1", "message-1", "same text")
	request.Memory.Messages = append(request.Memory.Messages, mekingmcp.MessageRequest{
		ID:   "message-2",
		Role: "assistant",
		Text: "same text",
	})
	first := callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", request)
	if len(first.Messages) != 2 || first.Messages[0].Position != 0 || first.Messages[1].Position != 1 {
		t.Fatalf("first Message positions = %#v", first.Messages)
	}
	if first.Messages[0].TextUnitID != first.Messages[1].TextUnitID {
		t.Fatalf("equal Message bodies have different TextUnit IDs: %#v", first.Messages)
	}

	second := callTool[mekingmcp.MemoryReceipt](
		t,
		fixture.client,
		"add_memory",
		memoryRequest(sessionID, "memory-2", "message-3", "same text"),
	)
	if len(second.Messages) != 1 || second.Messages[0].Position != 2 ||
		second.Messages[0].TextUnitID != first.Messages[0].TextUnitID {
		t.Fatalf("second Message = %#v", second.Messages)
	}
}

func assertSchemaError(t *testing.T, result *protocol.CallToolResult, path string) {
	t.Helper()
	if !result.IsError {
		t.Fatalf("schema validation result = %#v", result)
	}
	message := toolResultText(t, result)
	if !strings.Contains(message, strings.TrimPrefix(path, "/")) || !strings.Contains(message, "validation") {
		t.Fatalf("schema validation error = %q", message)
	}
}

func assertToolFailure(t *testing.T, result *protocol.CallToolResult, code string) {
	t.Helper()
	if !result.IsError {
		t.Fatalf("tool failure = %#v", result)
	}
	failure := structured[mekingmcp.ToolFailure](t, result.StructuredContent)
	if failure.Code != code {
		t.Fatalf("tool failure = %#v, want code %q", failure, code)
	}
}
