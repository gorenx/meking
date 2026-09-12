package mcp

import (
	"context"
	"fmt"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
)

type ClaimConflictRequest struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	ClaimID   string `json:"claim_id"`
}

type ClaimDecision struct {
	UserID    string      `json:"user_id"`
	SessionID string      `json:"session_id"`
	SourceID  string      `json:"source_id"`
	Choice    ClaimChoice `json:"decision"`
}

type ClaimChoice struct {
	BaseVersion  uint64              `json:"base_version"`
	Final        Claim               `json:"final"`
	Alternatives []ConflictReference `json:"alternatives"`
}

func (toolset *Toolset) registerClaimConflictTools(protocolServer *server.MCPServer) {
	protocolServer.AddTool(protocol.NewTool(
		"list_claim_conflicts",
		protocol.WithDescription("List pending Claim conflicts in one conversation Child Zone."),
		inputSchema[ConflictPageRequest](),
		outputSchema[Page[ClaimConflict]](),
		protocol.WithReadOnlyHintAnnotation(true),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.listClaimConflicts))
	protocolServer.AddTool(protocol.NewTool(
		"get_claim_conflict",
		protocol.WithDescription("Read the formal Claim, all conflicting revisions, and their original evidence."),
		inputSchema[ClaimConflictRequest](),
		outputSchema[ClaimConflict](),
		protocol.WithReadOnlyHintAnnotation(true),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.getClaimConflict))
	protocolServer.AddTool(protocol.NewTool(
		"resolve_claim_conflict",
		protocol.WithDescription("Record the complete final Claim for the current conflict without invoking a model."),
		inputSchema[ClaimDecision](),
		outputSchema[VersionChange](),
		protocol.WithReadOnlyHintAnnotation(false),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithIdempotentHintAnnotation(true),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.resolveClaimConflict))
}

func (toolset *Toolset) listClaimConflicts(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request ConflictPageRequest,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	page, err := toolset.Conflicts.BrowseClaimConflicts(bound, resolution.ConflictPageRequest{
		After: request.After,
		Limit: request.Limit,
	})
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(conflictPage(page, claimConflict)), nil
}

func (toolset *Toolset) getClaimConflict(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request ClaimConflictRequest,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	value, err := toolset.Conflicts.ClaimConflict(bound, knowledge.ClaimID(request.ClaimID))
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(claimConflict(value)), nil
}

func (toolset *Toolset) resolveClaimConflict(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request ClaimDecision,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	subject, err := subjectFromValue(request.Choice.Final.Subject)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	expectations, err := conflictExpectations(
		knowledge.ClaimID(request.Choice.Final.ID),
		knowledge.Version(request.Choice.BaseVersion),
		request.Choice.Alternatives,
	)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	result, err := toolset.Conflicts.ResolveClaim(bound, resolution.ClaimCommand{
		Source: provenance.Source{
			ID:         request.SourceID,
			Kind:       provenance.Resolution,
			ProducerID: request.SourceID,
		},
		BaseVersion: knowledge.Version(request.Choice.BaseVersion),
		Final: knowledge.Claim{
			ID:          knowledge.ClaimID(request.Choice.Final.ID),
			Subject:     subject,
			Type:        request.Choice.Final.Type,
			Description: request.Choice.Final.Description,
		},
		Candidates: expectations,
	})
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(VersionChange{
		Version: uint64(result.Version),
		Created: result.CreatedVersion,
	}), nil
}

func subjectFromValue(subject Subject) (knowledge.Subject, error) {
	switch subject.Kind {
	case "entity":
		return knowledge.NewEntitySubject(knowledge.EntityID(subject.ID))
	case "relation":
		return knowledge.NewRelationSubject(knowledge.RelationID(subject.ID))
	default:
		return nil, fmt.Errorf("%w: Claim subject kind must be entity or relation", knowledge.ErrInvalidChange)
	}
}
