package mcp

import (
	"context"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/deletion"
	"github.com/memoria-space/meking/knowledge/provenance"
)

type DeleteEntityRequest struct {
	UserID    string                                  `json:"user_id"`
	SessionID string                                  `json:"session_id"`
	SourceID  string                                  `json:"source_id"`
	Target    knowledge.Reference[knowledge.EntityID] `json:"target"`
}

type DeleteRelationRequest struct {
	UserID    string                                    `json:"user_id"`
	SessionID string                                    `json:"session_id"`
	SourceID  string                                    `json:"source_id"`
	Target    knowledge.Reference[knowledge.RelationID] `json:"target"`
}

type DeleteClaimRequest struct {
	UserID    string                                 `json:"user_id"`
	SessionID string                                 `json:"session_id"`
	SourceID  string                                 `json:"source_id"`
	Target    knowledge.Reference[knowledge.ClaimID] `json:"target"`
}

func (toolset *Toolset) registerDeletionTools(protocolServer *server.MCPServer) {
	protocolServer.AddTool(protocol.NewTool(
		"delete_entity",
		protocol.WithDescription("Create a tombstone for one exact current Entity version."),
		inputSchema[DeleteEntityRequest](),
		outputSchema[VersionChange](),
		protocol.WithReadOnlyHintAnnotation(false),
		protocol.WithDestructiveHintAnnotation(true),
		protocol.WithIdempotentHintAnnotation(true),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.deleteEntity))
	protocolServer.AddTool(protocol.NewTool(
		"delete_relation",
		protocol.WithDescription("Create a tombstone for one exact current Relation version."),
		inputSchema[DeleteRelationRequest](),
		outputSchema[VersionChange](),
		protocol.WithReadOnlyHintAnnotation(false),
		protocol.WithDestructiveHintAnnotation(true),
		protocol.WithIdempotentHintAnnotation(true),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.deleteRelation))
	protocolServer.AddTool(protocol.NewTool(
		"delete_claim",
		protocol.WithDescription("Create a tombstone for one exact current Claim version."),
		inputSchema[DeleteClaimRequest](),
		outputSchema[VersionChange](),
		protocol.WithReadOnlyHintAnnotation(false),
		protocol.WithDestructiveHintAnnotation(true),
		protocol.WithIdempotentHintAnnotation(true),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.deleteClaim))
}

func (toolset *Toolset) deleteEntity(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request DeleteEntityRequest,
) (*protocol.CallToolResult, error) {
	return toolset.deleteKnowledge(ctx, request.UserID, request.SessionID, request.SourceID, request.Target.ID, request.Target.Version)
}

func (toolset *Toolset) deleteRelation(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request DeleteRelationRequest,
) (*protocol.CallToolResult, error) {
	return toolset.deleteKnowledge(ctx, request.UserID, request.SessionID, request.SourceID, request.Target.ID, request.Target.Version)
}

func (toolset *Toolset) deleteClaim(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request DeleteClaimRequest,
) (*protocol.CallToolResult, error) {
	return toolset.deleteKnowledge(ctx, request.UserID, request.SessionID, request.SourceID, request.Target.ID, request.Target.Version)
}

func (toolset *Toolset) deleteKnowledge(
	ctx context.Context,
	userID string,
	sessionID string,
	sourceID string,
	target knowledge.ObjectRef,
	expectedVersion knowledge.Version,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, userID, sessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	result, err := toolset.Deletions.Delete(bound, deletion.Command{
		Source: provenance.Source{
			ID:         sourceID,
			Kind:       provenance.Deletion,
			ProducerID: sourceID,
		},
		Knowledge:       target,
		ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(VersionChange{
		Version: uint64(result.Version),
		Created: result.CreatedVersion,
	}), nil
}
