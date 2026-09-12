package assembly_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	protocol "github.com/mark3labs/mcp-go/mcp"
	protocolserver "github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/assembly"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/knowledge"
	mekingmcp "github.com/memoria-space/meking/mcp"
	"github.com/memoria-space/meking/memory"
	"github.com/memoria-space/meking/zone"
)

const mcpTestUserID = "user-1"

func TestMCPMemoryProtocol(t *testing.T) {
	fixture := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	service := fixture.service
	sessionID := fixture.sessionID
	protocolClient := fixture.client
	if fixture.protocolVersion != protocol.ProtocolVersion20260728 {
		t.Fatalf("protocol version = %q", fixture.protocolVersion)
	}
	discovery, err := protocolClient.Discover(t.Context(), protocol.DiscoverRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(discovery.SupportedVersions, protocol.ProtocolVersion20260728) {
		t.Fatalf("server/discover versions = %v", discovery.SupportedVersions)
	}

	tools, err := protocolClient.ListTools(t.Context(), protocol.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	toolNames := make([]string, len(tools.Tools))
	for index, tool := range tools.Tools {
		toolNames[index] = tool.Name
	}
	slices.Sort(toolNames)
	wantTools := []string{
		"add_memory",
		"delete_claim",
		"delete_entity",
		"delete_relation",
		"get_claim_conflict",
		"get_entity_conflict",
		"get_recall_evaluation_protocol",
		"get_relation_conflict",
		"list_claim_conflicts",
		"list_entity_conflicts",
		"list_relation_conflicts",
		"resolve_claim_conflict",
		"resolve_entity_conflict",
		"resolve_relation_conflict",
		"search_memory",
		"submit_recall_observation",
	}
	if !slices.Equal(toolNames, wantTools) {
		t.Fatalf("tools = %v, want %v", toolNames, wantTools)
	}
	templates, err := protocolClient.ListResourceTemplates(t.Context(), protocol.ListResourceTemplatesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	templateNames := make([]string, len(templates.ResourceTemplates))
	for index, template := range templates.ResourceTemplates {
		templateNames[index] = template.Name
	}
	slices.Sort(templateNames)
	wantTemplates := []string{"claim", "entity", "message", "relation"}
	if !slices.Equal(templateNames, wantTemplates) {
		t.Fatalf("resource templates = %v, want %v", templateNames, wantTemplates)
	}

	added := callTool[mekingmcp.MemoryReceipt](t, protocolClient, "add_memory", memoryRequest(
		string(sessionID),
		"memory-1",
		"message-1",
		"Alice is an engineer.",
	))
	if added.MemoryID != "memory-1" || len(added.Messages) != 1 {
		t.Fatalf("add_memory result = %#v", added)
	}
	if len(added.Conflicts.Entities) != 0 ||
		len(added.Conflicts.Relations) != 0 ||
		len(added.Conflicts.Claims) != 0 {
		t.Fatalf("add_memory conflicts = %#v", added.Conflicts)
	}
	root, err := service.Zones().Root(t.Context(), mcpTestUserID)
	if err != nil {
		t.Fatal(err)
	}
	if parentID, err := service.Zones().ResolveDirectParent(t.Context(), sessionID); err != nil || parentID != root.ID {
		t.Fatalf("automatically created Session Parent = %q, %v; want %q", parentID, err, root.ID)
	}

	found := callTool[mekingmcp.MemoryMatches](t, protocolClient, "search_memory", mekingmcp.SearchMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: string(sessionID),
		Query: memory.Query{
			Title: "ALICE",
			Type:  "PERSON",
		},
	})
	if len(found.Entities) != 1 ||
		found.Entities[0].Reason != "exact_identity" ||
		found.Entities[0].Version.Content.ID == "" ||
		len(found.Messages) != 1 ||
		found.Messages[0].MessageID != "message-1" {
		t.Fatalf("search_memory result = %#v", found)
	}
	entityID := found.Entities[0].Version.Content.ID
	semantic := callTool[mekingmcp.MemoryMatches](t, protocolClient, "search_memory", mekingmcp.SearchMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: string(sessionID),
		Query: memory.Query{
			Title: "engineer",
			Fuzzy: true,
		},
	})
	if len(semantic.Entities) != 1 || semantic.Entities[0].Reason != "semantic_match" ||
		semantic.Entities[0].Version.Content.ID != entityID {
		t.Fatalf("semantic search_memory result = %#v", semantic)
	}

	readResource[mekingmcp.MessageResource](
		t,
		protocolClient,
		fmt.Sprintf("meking://users/%s/sessions/%s/messages/message-1", mcpTestUserID, sessionID),
	)
	entityResource := readResource[mekingmcp.EntityResource](
		t,
		protocolClient,
		fmt.Sprintf(
			"meking://users/%s/sessions/%s/knowledge/entities/%s",
			mcpTestUserID,
			sessionID,
			entityID,
		),
	)
	if entityResource.Version.Content.ID != entityID || len(entityResource.Evidence) != 1 {
		t.Fatalf("Entity resource = %#v", entityResource)
	}

	conflicting := callTool[mekingmcp.MemoryReceipt](t, protocolClient, "add_memory", memoryRequest(
		string(sessionID),
		"memory-2",
		"message-2",
		"Alice is a designer.",
	))
	if len(conflicting.Conflicts.Entities) != 1 ||
		len(conflicting.Conflicts.Entities[0].Alternatives) != 1 {
		t.Fatalf("conflicting add_memory result = %#v", conflicting)
	}
	conflict := callTool[mekingmcp.EntityConflict](t, protocolClient, "get_entity_conflict", mekingmcp.EntityConflictRequest{
		UserID:    mcpTestUserID,
		SessionID: string(sessionID),
		EntityID:  string(entityID),
	})
	if conflict.ID != string(entityID) || len(conflict.Alternatives) != 1 {
		t.Fatalf("get_entity_conflict result = %#v", conflict)
	}
	listed := callTool[mekingmcp.Page[mekingmcp.EntityConflict]](
		t,
		protocolClient,
		"list_entity_conflicts",
		mekingmcp.ConflictPageRequest{UserID: mcpTestUserID, SessionID: string(sessionID)},
	)
	if len(listed.Items) != 1 || listed.Items[0].ID != string(entityID) {
		t.Fatalf("list_entity_conflicts result = %#v", listed)
	}
	resolved := callTool[mekingmcp.VersionChange](
		t,
		protocolClient,
		"resolve_entity_conflict",
		mekingmcp.EntityDecision{
			UserID:    mcpTestUserID,
			SessionID: string(sessionID),
			SourceID:  "resolution-1",
			Choice: mekingmcp.EntityChoice{
				BaseVersion:  conflict.BaseVersion,
				Final:        conflict.Alternatives[0].Content,
				Alternatives: []mekingmcp.ConflictReference{conflict.Alternatives[0].Reference},
			},
		},
	)
	if resolved.Version != 2 || !resolved.Created {
		t.Fatalf("resolve_entity_conflict result = %#v", resolved)
	}
	listed = callTool[mekingmcp.Page[mekingmcp.EntityConflict]](
		t,
		protocolClient,
		"list_entity_conflicts",
		mekingmcp.ConflictPageRequest{UserID: mcpTestUserID, SessionID: string(sessionID)},
	)
	if len(listed.Items) != 0 {
		t.Fatalf("resolved Entity remains pending = %#v", listed)
	}
	assertToolFailure(t, rawToolCall(t, protocolClient, "get_entity_conflict", mekingmcp.EntityConflictRequest{
		UserID:    mcpTestUserID,
		SessionID: string(sessionID),
		EntityID:  string(entityID),
	}), "conflict_not_found")
	resolvedResource := readResource[mekingmcp.EntityResource](
		t,
		protocolClient,
		fmt.Sprintf(
			"meking://users/%s/sessions/%s/knowledge/entities/%s",
			mcpTestUserID,
			sessionID,
			entityID,
		),
	)
	if resolvedResource.Version.Content.Description != "Alice is a designer." {
		t.Fatalf("resolved Entity resource = %#v", resolvedResource)
	}

	duplicate := rawToolCall(t, protocolClient, "add_memory", memoryRequest(
		string(sessionID),
		"memory-1",
		"message-3",
		"Alice is an engineer.",
	))
	if !duplicate.IsError {
		t.Fatalf("duplicate add_memory result = %#v", duplicate)
	}
	failure := structured[mekingmcp.ToolFailure](t, duplicate.StructuredContent)
	if failure.Code != "memory_already_exists" {
		t.Fatalf("duplicate error = %#v", failure)
	}
	sessionContext, err := zone.NewContext(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Messages().Read(sessionContext, []string{"message-3"}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("duplicate Memory appended Message: %v", err)
	}

	secondSessionID, err := zone.NewID()
	if err != nil {
		t.Fatal(err)
	}
	secondSessionReceipt := callTool[mekingmcp.MemoryReceipt](
		t,
		protocolClient,
		"add_memory",
		memoryRequest(string(secondSessionID), "memory-1", "message-1", "Alice is an engineer."),
	)
	if secondSessionReceipt.MemoryID != "memory-1" || len(secondSessionReceipt.Messages) != 1 ||
		secondSessionReceipt.Messages[0].Position != 0 {
		t.Fatalf("same Memory ID in another Session = %#v", secondSessionReceipt)
	}
	assertToolFailure(t, rawToolCall(t, protocolClient, "search_memory", mekingmcp.SearchMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: "not-a-zone-id",
		Query:     memory.Query{Title: "ALICE"},
	}), "invalid_session")

	otherRoot, err := service.Zones().Root(t.Context(), "user-2")
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := service.Zones().CreateChild(t.Context(), otherRoot.ID)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := rawToolCall(t, protocolClient, "search_memory", map[string]any{
		"user_id":    mcpTestUserID,
		"session_id": string(otherSession.ID),
		"query": map[string]any{
			"title":  "ALICE",
			"limits": map[string]any{},
		},
	})
	if !forbidden.IsError {
		t.Fatalf("cross-root search result = %#v", forbidden)
	}
	if failure := structured[mekingmcp.ToolFailure](t, forbidden.StructuredContent); failure.Code != "forbidden" {
		t.Fatalf("cross-root error = %#v", failure)
	}
	assertToolFailure(t, rawToolCall(t, protocolClient, "resolve_entity_conflict", mekingmcp.EntityDecision{
		UserID:    mcpTestUserID,
		SessionID: string(otherSession.ID),
		SourceID:  "forbidden-resolution",
		Choice: mekingmcp.EntityChoice{
			BaseVersion:  2,
			Final:        resolvedResource.Version.Content,
			Alternatives: []mekingmcp.ConflictReference{},
		},
	}), "forbidden")
	assertToolFailure(t, rawToolCall(t, protocolClient, "delete_entity", mekingmcp.DeleteEntityRequest{
		UserID:    mcpTestUserID,
		SessionID: string(otherSession.ID),
		SourceID:  "forbidden-deletion",
		Target: knowledge.Reference[knowledge.EntityID]{
			ID: entityID, Version: 2,
		},
	}), "forbidden")
}

type mcpFixture struct {
	service         *assembly.Service
	server          *protocolserver.MCPServer
	client          *client.Client
	sessionID       zone.ID
	protocolVersion string
}

func openMCPFixture(t *testing.T, protocolVersion string) mcpFixture {
	t.Helper()
	models := newAssemblyModelServer(t)
	service, err := assembly.Open(t.Context(), serviceConfigWithModelURL(t, t.TempDir(), models.URL))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close Service: %v", err)
		}
	})
	sessionID, err := zone.NewID()
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := service.MCP()
	if err != nil {
		t.Fatal(err)
	}
	protocolServer := toolset.Server()
	protocolClient, err := client.NewInProcessClient(protocolServer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := protocolClient.Close(); err != nil {
			t.Errorf("close MCP Client: %v", err)
		}
	})
	if err := protocolClient.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	initialize := protocol.InitializeRequest{}
	initialize.Params.ProtocolVersion = protocolVersion
	initialize.Params.ClientInfo = protocol.Implementation{
		Name:    "meking-integration-test",
		Version: "1",
	}
	initialized, err := protocolClient.Initialize(t.Context(), initialize)
	if err != nil {
		t.Fatal(err)
	}
	return mcpFixture{
		service:         service,
		server:          protocolServer,
		client:          protocolClient,
		sessionID:       sessionID,
		protocolVersion: initialized.ProtocolVersion,
	}
}

func memoryRequest(
	sessionID string,
	memoryID string,
	messageID string,
	description string,
) mekingmcp.AddMemoryRequest {
	return mekingmcp.AddMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: sessionID,
		Memory: mekingmcp.MemoryRequest{
			ID: memoryID,
			Messages: []mekingmcp.MessageRequest{
				{
					ID:   messageID,
					Role: "user",
					Text: description,
				},
			},
			Knowledge: mekingmcp.KnowledgeRequest{
				Entities: []mekingmcp.EntityMemory{
					{
						Content: knowledge.EntityContent{
							Identity: knowledge.EntityIdentity{
								Title: "ALICE",
								Type:  "PERSON",
							},
							Aliases:     []string{"Alice"},
							Description: description,
						},
						Source: mekingmcp.EntitySource{
							Frequency: 1,
						},
					},
				},
				Relations: []mekingmcp.RelationMemory{},
				Claims:    []mekingmcp.ClaimMemory{},
			},
		},
	}
}

func callTool[T any](
	t *testing.T,
	protocolClient *client.Client,
	name string,
	arguments any,
) T {
	t.Helper()
	result := rawToolCall(t, protocolClient, name, arguments)
	if result.IsError {
		t.Fatalf("%s result = %#v", name, result)
	}
	return structured[T](t, result.StructuredContent)
}

func rawToolCall(
	t *testing.T,
	protocolClient *client.Client,
	name string,
	arguments any,
) *protocol.CallToolResult {
	t.Helper()
	request := protocol.CallToolRequest{}
	request.Params.Name = name
	request.Params.Arguments = arguments
	result, err := protocolClient.CallTool(t.Context(), request)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return result
}

func readResource[T any](
	t *testing.T,
	protocolClient *client.Client,
	uri string,
) T {
	t.Helper()
	request := protocol.ReadResourceRequest{}
	request.Params.URI = uri
	result, err := protocolClient.ReadResource(t.Context(), request)
	if err != nil {
		t.Fatalf("read resource %q: %v", uri, err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("resource %q contents = %#v", uri, result.Contents)
	}
	content, ok := result.Contents[0].(protocol.TextResourceContents)
	if !ok {
		t.Fatalf("resource %q content type = %T", uri, result.Contents[0])
	}
	return structured[T](t, json.RawMessage(content.Text))
}

func structured[T any](t *testing.T, value any) T {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result T
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
