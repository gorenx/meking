package mcp

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/knowledge"
)

type ConflictPageRequest struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	After     string `json:"after,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type Page[T any] struct {
	Items     []T    `json:"items"`
	NextAfter string `json:"next_after"`
	HasMore   bool   `json:"has_more"`
}

func (toolset *Toolset) registerConflictTools(protocolServer *server.MCPServer) {
	toolset.registerEntityConflictTools(protocolServer)
	toolset.registerRelationConflictTools(protocolServer)
	toolset.registerClaimConflictTools(protocolServer)
}

func conflictPage[Input any, Output any, Cursor ~string](
	page knowledge.Page[Input, Cursor],
	convert func(Input) Output,
) Page[Output] {
	result := Page[Output]{
		Items:     make([]Output, len(page.Items)),
		NextAfter: string(page.NextAfter),
		HasMore:   page.HasMore,
	}
	for index, item := range page.Items {
		result.Items[index] = convert(item)
	}
	return result
}
