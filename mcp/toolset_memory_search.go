package mcp

import (
	"context"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/memory"
)

type SearchMemoryRequest struct {
	UserID    string       `json:"user_id"`
	SessionID string       `json:"session_id"`
	Query     memory.Query `json:"query"`
}

type EntityMatch struct {
	Version  Version[knowledge.Entity] `json:"version"`
	Score    float64                   `json:"score"`
	Reason   string                    `json:"reason"`
	Evidence []EntityEvidence          `json:"evidence"`
}

type RelationMatch struct {
	Version  Version[knowledge.Relation] `json:"version"`
	Reason   string                      `json:"reason"`
	Evidence []RelationEvidence          `json:"evidence"`
}

type ClaimMatch struct {
	Version  Version[Claim]  `json:"version"`
	Reason   string          `json:"reason"`
	Evidence []ClaimEvidence `json:"evidence"`
}

type MessageEvidence struct {
	ZoneID     string `json:"zone_id"`
	TextUnitID string `json:"text_unit_id"`
	MessageID  string `json:"message_id"`
	Position   uint64 `json:"position"`
	Role       string `json:"role"`
	Text       string `json:"text"`
}

type CorporaEvidence struct {
	ZoneID           string `json:"zone_id"`
	TextUnitID       string `json:"text_unit_id"`
	CorporaID        string `json:"corpora_id"`
	TextID           string `json:"text_id"`
	TextTitle        string `json:"text_title"`
	DocumentID       string `json:"document_id"`
	DocumentLocation string `json:"document_location"`
	StartIndex       int    `json:"start_index"`
	EndIndex         int    `json:"end_index"`
	TokenCount       int    `json:"token_count"`
	Text             string `json:"text"`
}

type MemoryMatches struct {
	Entities  []EntityMatch     `json:"entities"`
	Relations []RelationMatch   `json:"relations"`
	Claims    []ClaimMatch      `json:"claims"`
	Messages  []MessageEvidence `json:"messages"`
	Corpora   []CorporaEvidence `json:"corpora"`
	Truncated bool              `json:"truncated"`
}

func (toolset *Toolset) registerSearchMemory(protocolServer *server.MCPServer) {
	tool := protocol.NewTool(
		"search_memory",
		protocol.WithDescription("Search formal Entities by title or title and type, use semantic search on exact miss or fuzzy requests, and return original evidence."),
		inputSchema[SearchMemoryRequest](),
		outputSchema[MemoryMatches](),
		protocol.WithReadOnlyHintAnnotation(true),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	)
	protocolServer.AddTool(tool, protocol.NewTypedToolHandler(toolset.searchMemory))
}

func (toolset *Toolset) searchMemory(
	ctx context.Context,
	_ protocol.CallToolRequest,
	request SearchMemoryRequest,
) (*protocol.CallToolResult, error) {
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	result, err := toolset.Search.Search(bound, request.Query)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(memoryMatches(result)), nil
}

func memoryMatches(value memory.SearchResult) MemoryMatches {
	result := MemoryMatches{
		Entities:  make([]EntityMatch, len(value.Entities)),
		Relations: make([]RelationMatch, len(value.Relations)),
		Claims:    make([]ClaimMatch, len(value.Claims)),
		Messages:  make([]MessageEvidence, 0),
		Corpora:   make([]CorporaEvidence, 0),
		Truncated: value.Truncated,
	}
	for index, match := range value.Entities {
		result.Entities[index] = EntityMatch{
			Version:  entityVersion(match.Version),
			Score:    match.Score,
			Reason:   match.Reason,
			Evidence: entityEvidence(match.Evidence),
		}
	}
	for index, match := range value.Relations {
		result.Relations[index] = RelationMatch{
			Version:  relationVersion(match.Version),
			Reason:   match.Reason,
			Evidence: relationEvidence(match.Evidence),
		}
	}
	for index, match := range value.Claims {
		result.Claims[index] = ClaimMatch{
			Version:  claimVersion(match.Version),
			Reason:   match.Reason,
			Evidence: claimEvidence(match.Evidence),
		}
	}
	for _, evidence := range value.Evidence {
		switch source := evidence.Source.(type) {
		case memory.MessageEvidence:
			result.Messages = append(result.Messages, MessageEvidence{
				ZoneID:     evidence.Reference.ZoneID,
				TextUnitID: evidence.Reference.TextUnitID,
				MessageID:  source.Occurrence.Message.ID,
				Position:   source.Occurrence.Position,
				Role:       source.Occurrence.Message.Role,
				Text:       source.Occurrence.Message.TextUnit.Text,
			})
		case memory.CorporaEvidence:
			location := source.Location
			result.Corpora = append(result.Corpora, CorporaEvidence{
				ZoneID:           evidence.Reference.ZoneID,
				TextUnitID:       evidence.Reference.TextUnitID,
				CorporaID:        string(location.CorporaID),
				TextID:           string(location.TextID),
				TextTitle:        location.TextTitle,
				DocumentID:       string(location.DocumentID),
				DocumentLocation: string(location.DocumentLocation),
				StartIndex:       location.TextUnit.StartIndex,
				EndIndex:         location.TextUnit.EndIndex,
				TokenCount:       location.TextUnit.TokenCount,
				Text:             location.TextUnit.TextUnit.Text,
			})
		}
	}
	return result
}
