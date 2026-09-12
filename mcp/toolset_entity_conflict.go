package mcp

import (
	"context"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
)

type EntityConflictRequest struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	EntityID  string `json:"entity_id"`
}

type EntityDecision struct {
	UserID    string       `json:"user_id"`
	SessionID string       `json:"session_id"`
	SourceID  string       `json:"source_id"`
	Choice    EntityChoice `json:"decision"`
}

type EntityChoice struct {
	BaseVersion  uint64              `json:"base_version"`
	Final        knowledge.Entity    `json:"final"`
	Alternatives []ConflictReference `json:"alternatives"`
}

func (toolset *Toolset) registerEntityConflictTools(protocolServer *server.MCPServer) {
	protocolServer.AddTool(protocol.NewTool(
		"list_entity_conflicts",
		protocol.WithDescription("List pending Entity conflicts in one conversation Child Zone."),
		inputSchema[ConflictPageRequest](),
		outputSchema[Page[EntityConflict]](),
		protocol.WithReadOnlyHintAnnotation(true),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.listEntityConflicts))

	protocolServer.AddTool(protocol.NewTool(
		"get_entity_conflict",
		protocol.WithDescription("Read the formal Entity, all conflicting revisions, and their original evidence."),
		inputSchema[EntityConflictRequest](),
		outputSchema[EntityConflict](),
		protocol.WithReadOnlyHintAnnotation(true),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.getEntityConflict))

	protocolServer.AddTool(protocol.NewTool(
		"resolve_entity_conflict",
		protocol.WithDescription("Record the complete final Entity for the current conflict without invoking a model."),
		inputSchema[EntityDecision](),
		outputSchema[VersionChange](),
		protocol.WithReadOnlyHintAnnotation(false),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithIdempotentHintAnnotation(true),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.resolveEntityConflict))
}

func (toolset *Toolset) listEntityConflicts(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request ConflictPageRequest,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	page, err := toolset.Conflicts.BrowseEntityConflicts(bound, resolution.ConflictPageRequest{
		After: request.After,
		Limit: request.Limit,
	})
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(conflictPage(page, entityConflict)), nil
}

func (toolset *Toolset) getEntityConflict(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request EntityConflictRequest,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	value, err := toolset.Conflicts.EntityConflict(bound, knowledge.EntityID(request.EntityID))
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(entityConflict(value)), nil
}

func (toolset *Toolset) resolveEntityConflict(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request EntityDecision,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	expectations, err := conflictExpectations(
		request.Choice.Final.ID,
		knowledge.Version(request.Choice.BaseVersion),
		request.Choice.Alternatives,
	)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	result, err := toolset.Conflicts.ResolveEntity(bound, resolution.EntityCommand{
		Source: provenance.Source{
			ID:         request.SourceID,
			Kind:       provenance.Resolution,
			ProducerID: request.SourceID,
		},
		BaseVersion: knowledge.Version(request.Choice.BaseVersion),
		Final:       request.Choice.Final,
		Candidates:  expectations,
	})
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(VersionChange{
		Version: uint64(result.Version),
		Created: result.CreatedVersion,
	}), nil
}
