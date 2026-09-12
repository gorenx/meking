package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/knowledge"
)

const resourceMIMEType = "application/json"

type MessageResource struct {
	UserID     string `json:"user_id"`
	SessionID  string `json:"session_id"`
	MessageID  string `json:"message_id"`
	Position   uint64 `json:"position"`
	Role       string `json:"role"`
	TextUnitID string `json:"text_unit_id"`
	Text       string `json:"text"`
}

type EntityResource struct {
	Version  Version[knowledge.Entity] `json:"version"`
	Evidence []EntityEvidence          `json:"evidence"`
}

type RelationResource struct {
	Version  Version[knowledge.Relation] `json:"version"`
	Evidence []RelationEvidence          `json:"evidence"`
}

type ClaimResource struct {
	Version  Version[Claim]  `json:"version"`
	Evidence []ClaimEvidence `json:"evidence"`
}

func (toolset *Toolset) registerResources(protocolServer *server.MCPServer) {
	protocolServer.AddResourceTemplate(
		protocol.NewResourceTemplate(
			"meking://users/{user_id}/sessions/{session_id}/messages/{message_id}",
			"message",
			protocol.WithTemplateDescription("Read one immutable message from its Session Zone."),
			protocol.WithTemplateMIMEType(resourceMIMEType),
		),
		toolset.readMessageResource,
	)
	protocolServer.AddResourceTemplate(
		protocol.NewResourceTemplate(
			"meking://users/{user_id}/sessions/{session_id}/knowledge/entities/{entity_id}",
			"entity",
			protocol.WithTemplateDescription("Read the latest formal Entity and its evidence."),
			protocol.WithTemplateMIMEType(resourceMIMEType),
		),
		toolset.readEntityResource,
	)
	protocolServer.AddResourceTemplate(
		protocol.NewResourceTemplate(
			"meking://users/{user_id}/sessions/{session_id}/knowledge/relations/{relation_id}",
			"relation",
			protocol.WithTemplateDescription("Read the latest formal Relation and its evidence."),
			protocol.WithTemplateMIMEType(resourceMIMEType),
		),
		toolset.readRelationResource,
	)
	protocolServer.AddResourceTemplate(
		protocol.NewResourceTemplate(
			"meking://users/{user_id}/sessions/{session_id}/knowledge/claims/{claim_id}",
			"claim",
			protocol.WithTemplateDescription("Read the latest formal Claim and its evidence."),
			protocol.WithTemplateMIMEType(resourceMIMEType),
		),
		toolset.readClaimResource,
	)
}

func (toolset *Toolset) readMessageResource(
	ctx context.Context,
	request protocol.ReadResourceRequest,
) ([]protocol.ResourceContents, error) {
	address, err := parseResourceAddress(request.Params.URI, "messages")
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	bound, err := toolset.zoneContext(ctx, address.UserID, address.SessionID)
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	occurrences, err := toolset.Messages.Read(bound, []string{address.ObjectID})
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	if len(occurrences) != 1 || occurrences[0].Message.ID != address.ObjectID {
		return nil, toolset.resourceFailure(ctx, errors.New("Message read returned a different result"))
	}
	return resourceContents(
		request.Params.URI,
		messageResource(address.UserID, address.SessionID, occurrences[0]),
	)
}

func messageResource(userID string, sessionID string, occurrence message.Occurrence) MessageResource {
	return MessageResource{
		UserID:     userID,
		SessionID:  sessionID,
		MessageID:  occurrence.Message.ID,
		Position:   occurrence.Position,
		Role:       occurrence.Message.Role,
		TextUnitID: string(occurrence.Message.TextUnit.ID),
		Text:       occurrence.Message.TextUnit.Text,
	}
}

func (toolset *Toolset) readEntityResource(
	ctx context.Context,
	request protocol.ReadResourceRequest,
) ([]protocol.ResourceContents, error) {
	address, err := parseResourceAddress(request.Params.URI, "knowledge", "entities")
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	bound, err := toolset.zoneContext(ctx, address.UserID, address.SessionID)
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	version, found, err := toolset.Versions.CurrentEntity(bound, knowledge.EntityID(address.ObjectID))
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	if !found {
		return nil, toolset.resourceFailure(ctx, knowledge.ErrNotFound)
	}
	reference := knowledge.Reference[knowledge.EntityID]{
		ID:      version.Knowledge.ID,
		Version: version.Version,
	}
	evidence, err := toolset.Evidence.ReadEntityEvidence(bound, reference)
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	return resourceContents(request.Params.URI, EntityResource{
		Version:  entityVersion(version),
		Evidence: entityEvidence(evidence),
	})
}

func (toolset *Toolset) readRelationResource(
	ctx context.Context,
	request protocol.ReadResourceRequest,
) ([]protocol.ResourceContents, error) {
	address, err := parseResourceAddress(request.Params.URI, "knowledge", "relations")
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	bound, err := toolset.zoneContext(ctx, address.UserID, address.SessionID)
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	version, found, err := toolset.Versions.CurrentRelation(bound, knowledge.RelationID(address.ObjectID))
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	if !found {
		return nil, toolset.resourceFailure(ctx, knowledge.ErrNotFound)
	}
	reference := knowledge.Reference[knowledge.RelationID]{
		ID:      version.Knowledge.ID,
		Version: version.Version,
	}
	evidence, err := toolset.Evidence.ReadRelationEvidence(bound, reference)
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	return resourceContents(request.Params.URI, RelationResource{
		Version:  relationVersion(version),
		Evidence: relationEvidence(evidence),
	})
}

func (toolset *Toolset) readClaimResource(
	ctx context.Context,
	request protocol.ReadResourceRequest,
) ([]protocol.ResourceContents, error) {
	address, err := parseResourceAddress(request.Params.URI, "knowledge", "claims")
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	bound, err := toolset.zoneContext(ctx, address.UserID, address.SessionID)
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	version, found, err := toolset.Versions.CurrentClaim(bound, knowledge.ClaimID(address.ObjectID))
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	if !found {
		return nil, toolset.resourceFailure(ctx, knowledge.ErrNotFound)
	}
	reference := knowledge.Reference[knowledge.ClaimID]{
		ID:      version.Knowledge.ID,
		Version: version.Version,
	}
	evidence, err := toolset.Evidence.ReadClaimEvidence(bound, reference)
	if err != nil {
		return nil, toolset.resourceFailure(ctx, err)
	}
	return resourceContents(request.Params.URI, ClaimResource{
		Version:  claimVersion(version),
		Evidence: claimEvidence(evidence),
	})
}

type resourceAddress struct {
	UserID    string
	SessionID string
	ObjectID  string
}

func parseResourceAddress(uri string, path ...string) (resourceAddress, error) {
	parsed, err := url.ParseRequestURI(uri)
	if err != nil || parsed.Scheme != "meking" || parsed.Host != "users" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return resourceAddress{}, errors.New("invalid Meking resource URI")
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	want := 4 + len(path)
	if len(segments) != want {
		return resourceAddress{}, errors.New("invalid Meking resource path")
	}
	values := make([]string, len(segments))
	for index, segment := range segments {
		value, err := url.PathUnescape(segment)
		if err != nil || value == "" || strings.Contains(value, "/") {
			return resourceAddress{}, errors.New("invalid Meking resource identifier")
		}
		values[index] = value
	}
	address := resourceAddress{
		UserID:    values[0],
		SessionID: values[2],
		ObjectID:  values[len(values)-1],
	}
	if values[1] != "sessions" {
		return resourceAddress{}, errors.New("invalid Meking resource path")
	}
	switch len(path) {
	case 1:
		if values[3] != path[0] {
			return resourceAddress{}, errors.New("invalid Meking resource path")
		}
	case 2:
		if values[3] != path[0] || values[4] != path[1] {
			return resourceAddress{}, errors.New("invalid Meking resource path")
		}
		address.ObjectID = values[5]
	default:
		return resourceAddress{}, errors.New("unsupported Meking resource path")
	}
	return address, nil
}

func resourceContents(uri string, value any) ([]protocol.ResourceContents, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode MCP resource: %w", err)
	}
	return []protocol.ResourceContents{
		protocol.TextResourceContents{
			URI:      uri,
			MIMEType: resourceMIMEType,
			Text:     string(encoded),
		},
	}, nil
}

func (toolset *Toolset) resourceFailure(ctx context.Context, err error) error {
	failure := classifyFailure(err)
	if failure.Code == "internal_error" {
		toolset.Logger.ErrorContext(ctx, "MCP resource failed", "error", err)
	}
	return fmt.Errorf("%s: %s", failure.Code, failure.Message)
}
