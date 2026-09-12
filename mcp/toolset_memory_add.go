package mcp

import (
	"context"
	"fmt"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/memory"
)

const (
	maximumMessagesPerMemory = 128
	maximumKnowledgePerKind  = 512
)

type AddMemoryRequest struct {
	UserID    string        `json:"user_id"`
	SessionID string        `json:"session_id"`
	Memory    MemoryRequest `json:"memory"`
}

type MemoryRequest struct {
	ID        string           `json:"id"`
	Messages  []MessageRequest `json:"messages"`
	Knowledge KnowledgeRequest `json:"knowledge"`
}

type MessageRequest struct {
	ID   string `json:"id"`
	Role string `json:"role"`
	Text string `json:"text"`
}

type KnowledgeRequest struct {
	Entities  []EntityMemory   `json:"entities,omitempty"`
	Relations []RelationMemory `json:"relations,omitempty"`
	Claims    []ClaimMemory    `json:"claims,omitempty"`
}

type ConflictSet struct {
	Entities  []EntityConflict   `json:"entities"`
	Relations []RelationConflict `json:"relations"`
	Claims    []ClaimConflict    `json:"claims"`
}

type MemoryReceipt struct {
	MemoryID  string            `json:"memory_id"`
	Messages  []MessageResource `json:"messages"`
	Conflicts ConflictSet       `json:"conflicts"`
}

func (toolset *Toolset) registerMemoryTools(protocolServer *server.MCPServer) {
	tool := protocol.NewTool(
		"add_memory",
		protocol.WithDescription("Save ordered messages and Agent-extracted knowledge in one User Session."),
		inputSchema[AddMemoryRequest](),
		outputSchema[MemoryReceipt](),
		protocol.WithReadOnlyHintAnnotation(false),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithIdempotentHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	)
	protocolServer.AddTool(tool, protocol.NewTypedToolHandler(toolset.addMemory))
	toolset.registerSearchMemory(protocolServer)
}

func (toolset *Toolset) addMemory(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request AddMemoryRequest,
) (*protocol.CallToolResult, error) {
	value, err := memoryValue(request.Memory)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	receipt, err := toolset.Memories.Add(bound, value)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(memoryReceipt(request.UserID, request.SessionID, receipt)), nil
}

func memoryValue(request MemoryRequest) (memory.Memory, error) {
	if len(request.Messages) == 0 || len(request.Messages) > maximumMessagesPerMemory {
		return memory.Memory{}, fmt.Errorf(
			"%w: Messages must contain between 1 and %d items",
			memory.ErrInvalid,
			maximumMessagesPerMemory,
		)
	}
	if len(request.Knowledge.Entities) > maximumKnowledgePerKind ||
		len(request.Knowledge.Relations) > maximumKnowledgePerKind ||
		len(request.Knowledge.Claims) > maximumKnowledgePerKind {
		return memory.Memory{}, fmt.Errorf(
			"%w: each Knowledge collection is limited to %d items",
			memory.ErrInvalid,
			maximumKnowledgePerKind,
		)
	}
	value := memory.Memory{
		ID:       request.ID,
		Messages: make([]message.Message, len(request.Messages)),
		Knowledge: memory.Knowledge{
			Entities:  make([]submission.Entity, len(request.Knowledge.Entities)),
			Relations: make([]submission.Relation, len(request.Knowledge.Relations)),
			Claims:    make([]submission.Claim, len(request.Knowledge.Claims)),
		},
	}
	for index, item := range request.Messages {
		created, err := message.New(item.ID, item.Role, item.Text)
		if err != nil {
			return memory.Memory{}, fmt.Errorf("%w: Message %d: %v", memory.ErrInvalid, index, err)
		}
		value.Messages[index] = created
	}
	for index, item := range request.Knowledge.Entities {
		value.Knowledge.Entities[index] = entityMemory(item)
	}
	for index, item := range request.Knowledge.Relations {
		value.Knowledge.Relations[index] = relationMemory(item)
	}
	for index, item := range request.Knowledge.Claims {
		created, err := claimMemory(item)
		if err != nil {
			return memory.Memory{}, fmt.Errorf("%w: Claim %d: %v", memory.ErrInvalid, index, err)
		}
		value.Knowledge.Claims[index] = created
	}
	if err := value.Validate(); err != nil {
		return memory.Memory{}, err
	}
	return value, nil
}

func memoryReceipt(userID string, sessionID string, receipt memory.Receipt) MemoryReceipt {
	result := MemoryReceipt{
		MemoryID: receipt.MemoryID,
		Messages: make([]MessageResource, len(receipt.Messages)),
		Conflicts: ConflictSet{
			Entities:  make([]EntityConflict, len(receipt.Conflicts.Entities)),
			Relations: make([]RelationConflict, len(receipt.Conflicts.Relations)),
			Claims:    make([]ClaimConflict, len(receipt.Conflicts.Claims)),
		},
	}
	for index, occurrence := range receipt.Messages {
		result.Messages[index] = messageResource(userID, sessionID, occurrence)
	}
	for index, value := range receipt.Conflicts.Entities {
		result.Conflicts.Entities[index] = entityConflict(value)
	}
	for index, value := range receipt.Conflicts.Relations {
		result.Conflicts.Relations[index] = relationConflict(value)
	}
	for index, value := range receipt.Conflicts.Claims {
		result.Conflicts.Claims[index] = claimConflict(value)
	}
	return result
}
