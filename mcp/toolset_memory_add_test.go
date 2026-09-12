package mcp

import (
	"errors"
	"testing"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/memory"
)

func TestMemoryRequiresKnowledge(t *testing.T) {
	request := MemoryRequest{
		ID: "memory-1",
		Messages: []MessageRequest{{
			ID:   "message-1",
			Role: "user",
			Text: "Alice is an engineer.",
		}},
	}
	if _, err := memoryValue(request); !errors.Is(err, memory.ErrInvalid) {
		t.Fatalf("memoryValue error = %v", err)
	}
}

func TestMemoryAcceptsAnyKnowledgeKind(t *testing.T) {
	for _, test := range []struct {
		name      string
		knowledge KnowledgeRequest
	}{
		{
			name: "Entity",
			knowledge: KnowledgeRequest{Entities: []EntityMemory{{
				Content: knowledge.EntityContent{
					Identity:    knowledge.EntityIdentity{Title: "Alice", Type: "Person"},
					Description: "Alice is an engineer.",
				},
			}}},
		},
		{
			name: "Relation",
			knowledge: KnowledgeRequest{Relations: []RelationMemory{{
				Content: knowledge.RelationContent{
					Source:      knowledge.EntityIdentity{Title: "Alice", Type: "Person"},
					Target:      knowledge.EntityIdentity{Title: "Acme", Type: "Organization"},
					Type:        "WorksAt",
					Description: "Alice works at Acme.",
				},
			}}},
		},
		{
			name: "Claim",
			knowledge: KnowledgeRequest{Claims: []ClaimMemory{{
				Subject: ClaimSubject{Entity: &knowledge.EntityIdentity{
					Title: "Alice",
					Type:  "Person",
				}},
				Type:        "Occupation",
				Description: "Alice is an engineer.",
			}}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := MemoryRequest{
				ID: "memory-1",
				Messages: []MessageRequest{{
					ID:   "message-1",
					Role: "user",
					Text: "Alice is an engineer.",
				}},
				Knowledge: test.knowledge,
			}
			if _, err := memoryValue(request); err != nil {
				t.Fatalf("memoryValue error = %v", err)
			}
		})
	}
}
