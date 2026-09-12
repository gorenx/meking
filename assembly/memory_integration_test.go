package assembly_test

import (
	"context"
	"errors"
	"testing"

	"github.com/memoria-space/meking/assembly"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/memory"
)

func TestMemoryRecorderAddsMessagesAndKnowledge(t *testing.T) {
	service, sessionContext := openMemoryService(t)
	value := testMemory(t, "memory-1", "message-1", "Alice works at Acme.", "Alice is a person.")

	receipt, err := service.MemoryRecorder().Add(sessionContext, value)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if receipt.MemoryID != value.ID || len(receipt.Messages) != 1 || receipt.Messages[0].Position != 0 {
		t.Fatalf("Memory Receipt = %#v", receipt)
	}
	if len(receipt.Submission.CreatedVersions.Entities) != 1 ||
		len(receipt.Conflicts.Entities) != 0 ||
		len(receipt.Conflicts.Relations) != 0 ||
		len(receipt.Conflicts.Claims) != 0 {
		t.Fatalf("Memory Receipt result = %#v", receipt)
	}
	stored, err := service.Messages().Read(sessionContext, []string{"message-1"})
	if err != nil {
		t.Fatal(err)
	}
	if stored[0].Message != value.Messages[0] || stored[0].Position != 0 {
		t.Fatalf("stored Message = %#v", stored[0])
	}
}

func TestMemoryRecorderReturnsKnowledgeConflict(t *testing.T) {
	service, sessionContext := openMemoryService(t)
	first := testMemory(t, "memory-1", "message-1", "Alice is an engineer.", "Alice is an engineer.")
	if _, err := service.MemoryRecorder().Add(sessionContext, first); err != nil {
		t.Fatal(err)
	}
	second := testMemory(t, "memory-2", "message-2", "Alice is a designer.", "Alice is a designer.")
	receipt, err := service.MemoryRecorder().Add(sessionContext, second)
	if err != nil {
		t.Fatalf("conflicting Add() error = %v", err)
	}
	if len(receipt.Conflicts.Entities) != 1 || len(receipt.Conflicts.Entities[0].Candidates) != 1 {
		t.Fatalf("Entity conflicts = %#v", receipt.Conflicts.Entities)
	}
	candidate := receipt.Conflicts.Entities[0].Candidates[0]
	if candidate.Candidate.Content.Description != "Alice is a designer." || len(candidate.Sources) != 1 {
		t.Fatalf("Entity Candidate = %#v", candidate)
	}
	evidence := candidate.Sources[0].Metadata.Evidence
	if len(evidence) != 1 {
		t.Fatalf("Candidate Evidence = %#v", evidence)
	}
	source, ok := evidence[0].Source.(provenance.MessageSource)
	if !ok || source.MessageID != "message-2" {
		t.Fatalf("Candidate Evidence Source = %#v", evidence[0].Source)
	}
}

func TestMemoryRecorderRejectsInvalidKnowledgeBeforeSavingMessage(t *testing.T) {
	service, sessionContext := openMemoryService(t)
	value := testMemory(t, "memory-1", "message-1", "Alice knows Bob.", "Alice is a person.")
	value.Knowledge.Relations = []submission.Relation{{
		Content: knowledge.RelationContent{
			Source: knowledge.EntityIdentity{Title: "ALICE", Type: "PERSON"},
			Target: knowledge.EntityIdentity{Title: "BOB", Type: "PERSON"},
			Type:   "KNOWS", Description: "Alice knows Bob.",
		},
		Metadata: provenance.RelationMetadata{Weight: 1},
	}}

	if _, err := service.MemoryRecorder().Add(sessionContext, value); err == nil {
		t.Fatal("Add() accepted Relation with an unknown target Entity")
	}
	if _, err := service.Messages().Read(sessionContext, []string{"message-1"}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("Message read after rejected Memory error = %v", err)
	}
}

func TestMemoryRecorderReusesDuplicateMessage(t *testing.T) {
	service, sessionContext := openMemoryService(t)
	value := testMemory(t, "memory-1", "message-1", "Alice is an engineer.", "Alice is an engineer.")
	if _, err := service.MemoryRecorder().Add(sessionContext, value); err != nil {
		t.Fatal(err)
	}
	value.ID = "memory-2"
	receipt, err := service.MemoryRecorder().Add(sessionContext, value)
	if err != nil {
		t.Fatalf("reuse Message error = %v", err)
	}
	if len(receipt.Messages) != 1 || receipt.Messages[0].Position != 0 {
		t.Fatalf("reused Message = %#v", receipt.Messages)
	}
}

func TestMemorySearcherReadsEntityByExactIdentity(t *testing.T) {
	service, sessionContext := openMemoryService(t)
	value := testMemory(t, "memory-1", "message-1", "Alice is an engineer.", "Alice is an engineer.")
	if _, err := service.MemoryRecorder().Add(sessionContext, value); err != nil {
		t.Fatal(err)
	}

	result, err := service.MemorySearcher().Search(sessionContext, memory.Query{
		Title: " ALICE ",
		Type:  " PERSON ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 1 ||
		result.Entities[0].Reason != "exact_identity" ||
		result.Entities[0].Version.Knowledge.Title != "ALICE" ||
		result.Entities[0].Version.Knowledge.Type != "PERSON" {
		t.Fatalf("Entity matches = %#v", result.Entities)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("Evidence = %#v", result.Evidence)
	}
	messageEvidence, ok := result.Evidence[0].Source.(memory.MessageEvidence)
	if !ok || messageEvidence.Occurrence.Message.ID != "message-1" {
		t.Fatalf("Message Evidence = %#v", result.Evidence[0].Source)
	}
}

func openMemoryService(t *testing.T) (*assembly.Service, context.Context) {
	t.Helper()
	service, err := assembly.Open(t.Context(), serviceConfig(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close Service: %v", err)
		}
	})
	root, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.Zones().CreateChild(t.Context(), root.ID)
	if err != nil {
		t.Fatal(err)
	}
	return service, bindZoneContext(t, session.ID)
}

func testMemory(t *testing.T, memoryID string, messageID string, text string, description string) memory.Memory {
	t.Helper()
	value, err := message.New(messageID, "user", text)
	if err != nil {
		t.Fatal(err)
	}
	return memory.Memory{
		ID:       memoryID,
		Messages: []message.Message{value},
		Knowledge: memory.Knowledge{Entities: []submission.Entity{{
			Content: knowledge.EntityContent{
				Identity: knowledge.EntityIdentity{Title: "ALICE", Type: "PERSON"},
				Aliases:  []string{"Alice"}, Description: description,
			},
			Metadata: provenance.EntityMetadata{Frequency: 1},
		}}},
	}
}
