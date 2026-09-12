package mcp

import (
	"context"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
)

type RelationConflictRequest struct {
	UserID     string `json:"user_id"`
	SessionID  string `json:"session_id"`
	RelationID string `json:"relation_id"`
}

type RelationDecision struct {
	UserID    string         `json:"user_id"`
	SessionID string         `json:"session_id"`
	SourceID  string         `json:"source_id"`
	Choice    RelationChoice `json:"decision"`
}

type RelationChoice struct {
	BaseVersion  uint64              `json:"base_version"`
	Final        knowledge.Relation  `json:"final"`
	Alternatives []ConflictReference `json:"alternatives"`
}

func (toolset *Toolset) registerRelationConflictTools(protocolServer *server.MCPServer) {
	protocolServer.AddTool(protocol.NewTool(
		"list_relation_conflicts",
		protocol.WithDescription("List pending Relation conflicts in one conversation Child Zone."),
		inputSchema[ConflictPageRequest](),
		outputSchema[Page[RelationConflict]](),
		protocol.WithReadOnlyHintAnnotation(true),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.listRelationConflicts))
	protocolServer.AddTool(protocol.NewTool(
		"get_relation_conflict",
		protocol.WithDescription("Read the formal Relation, all conflicting revisions, and their original evidence."),
		inputSchema[RelationConflictRequest](),
		outputSchema[RelationConflict](),
		protocol.WithReadOnlyHintAnnotation(true),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.getRelationConflict))
	protocolServer.AddTool(protocol.NewTool(
		"resolve_relation_conflict",
		protocol.WithDescription("Record the complete final Relation for the current conflict without invoking a model."),
		inputSchema[RelationDecision](),
		outputSchema[VersionChange](),
		protocol.WithReadOnlyHintAnnotation(false),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithIdempotentHintAnnotation(true),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.resolveRelationConflict))
}

func (toolset *Toolset) listRelationConflicts(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request ConflictPageRequest,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	page, err := toolset.Conflicts.BrowseRelationConflicts(bound, resolution.ConflictPageRequest{
		After: request.After,
		Limit: request.Limit,
	})
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(conflictPage(page, relationConflict)), nil
}

func (toolset *Toolset) getRelationConflict(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request RelationConflictRequest,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	value, err := toolset.Conflicts.RelationConflict(bound, knowledge.RelationID(request.RelationID))
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(relationConflict(value)), nil
}

func (toolset *Toolset) resolveRelationConflict(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request RelationDecision,
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
	result, err := toolset.Conflicts.ResolveRelation(bound, resolution.RelationCommand{
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
