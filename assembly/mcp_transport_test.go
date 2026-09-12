package assembly_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	protocol "github.com/mark3labs/mcp-go/mcp"
	protocolserver "github.com/mark3labs/mcp-go/server"
	mekingmcp "github.com/memoria-space/meking/mcp"
)

func TestMCPNegotiatesLegacyProtocol(t *testing.T) {
	fixture := openMCPFixture(t, protocol.ProtocolVersion20250326)
	if fixture.protocolVersion != protocol.ProtocolVersion20250326 {
		t.Fatalf("protocol version = %q", fixture.protocolVersion)
	}
	tools, err := fixture.client.ListTools(t.Context(), protocol.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 16 {
		t.Fatalf("tools = %d, want 16", len(tools.Tools))
	}
}

func TestMCPStdioListsMemoryTools(t *testing.T) {
	fixture := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	input := bytes.NewBufferString(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"stdio-test","version":"1"},"capabilities":{}}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n",
	)
	var output bytes.Buffer
	stdio := protocolserver.NewStdioServer(fixture.server)
	stdio.SetErrorLogger(log.New(io.Discard, "", 0))
	if err := stdio.Listen(t.Context(), input, &output); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("STDIO MCP: %v", err)
	}

	responses := make(map[float64]map[string]any)
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		var response map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			t.Fatalf("decode STDIO response %q: %v", scanner.Text(), err)
		}
		if id, ok := response["id"].(float64); ok {
			responses[id] = response
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if responses[1] == nil || responses[1]["error"] != nil {
		t.Fatalf("STDIO initialize response = %#v", responses[1])
	}
	listed := responses[2]
	if listed == nil || listed["error"] != nil {
		t.Fatalf("STDIO tools/list response = %#v", listed)
	}
	result, ok := listed["result"].(map[string]any)
	if !ok {
		t.Fatalf("STDIO tools/list result = %#v", listed["result"])
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) != 16 {
		t.Fatalf("STDIO tools = %#v", result["tools"])
	}
}

func TestMCPRecoversHandlerPanics(t *testing.T) {
	fixture := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	const secret = "panic payload must not reach the Agent"
	fixture.server.AddTool(
		protocol.NewTool("panic_tool"),
		func(context.Context, protocol.CallToolRequest) (*protocol.CallToolResult, error) {
			panic(secret)
		},
	)
	failed := rawToolCall(t, fixture.client, "panic_tool", map[string]any{})
	if !failed.IsError {
		t.Fatalf("panic tool result = %#v", failed)
	}
	failure := structured[mekingmcp.ToolFailure](t, failed.StructuredContent)
	if failure.Code != "internal_error" || !failure.Retryable {
		t.Fatalf("panic tool failure = %#v", failure)
	}
	if strings.Contains(toolResultText(t, failed), secret) {
		t.Fatal("panic tool response exposed the panic value")
	}

	const resourceURI = "meking-test://panic"
	fixture.server.AddResource(
		protocol.NewResource(resourceURI, "panic-resource"),
		func(context.Context, protocol.ReadResourceRequest) ([]protocol.ResourceContents, error) {
			panic(secret)
		},
	)
	request := protocol.ReadResourceRequest{}
	request.Params.URI = resourceURI
	if _, err := fixture.client.ReadResource(t.Context(), request); err == nil {
		t.Fatal("panic resource error = nil")
	} else if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "internal_error") {
		t.Fatalf("panic resource error = %v", err)
	}
	if _, err := fixture.client.ListTools(t.Context(), protocol.ListToolsRequest{}); err != nil {
		t.Fatalf("server did not continue after panic: %v", err)
	}
}

func TestMCPStreamableHTTPIsStatelessAndScopesResources(t *testing.T) {
	fixture := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	otherRoot, err := fixture.service.Zones().Root(t.Context(), "user-2")
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := fixture.service.Zones().CreateChild(t.Context(), otherRoot.ID)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle(
		"/api/v1/mcp",
		protocolserver.NewStreamableHTTPServer(
			fixture.server,
			protocolserver.WithStateLess(true),
		),
	)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	headers := &mcpHeaderCapture{transport: http.DefaultTransport}
	protocolClient, err := client.NewStreamableHttpClient(
		httpServer.URL+"/api/v1/mcp",
		transport.WithHTTPBasicClient(&http.Client{Transport: headers}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := protocolClient.Close(); err != nil {
			t.Errorf("close HTTP MCP Client: %v", err)
		}
	})
	if err := protocolClient.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	initialize := protocol.InitializeRequest{}
	initialize.Params.ProtocolVersion = protocol.ProtocolVersion20260728
	initialize.Params.ClientInfo = protocol.Implementation{Name: "http-test", Version: "1"}
	initialized, err := protocolClient.Initialize(t.Context(), initialize)
	if err != nil {
		t.Fatal(err)
	}
	if initialized.ProtocolVersion != protocol.ProtocolVersion20260728 {
		t.Fatalf("protocol version = %q", initialized.ProtocolVersion)
	}
	if headers.hasSessionID() {
		t.Fatal("stateless MCP exchanged an MCP Session ID")
	}

	request := protocol.ReadResourceRequest{}
	request.Params.URI = "meking://users/" + mcpTestUserID + "/sessions/" + string(otherSession.ID) + "/messages/missing"
	if _, err := protocolClient.ReadResource(t.Context(), request); err == nil {
		t.Fatal("cross-root Resource read error = nil")
	} else if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("cross-root Resource read error = %v", err)
	}
}

func toolResultText(t *testing.T, result *protocol.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("tool result content = %#v", result.Content)
	}
	content, ok := result.Content[0].(protocol.TextContent)
	if !ok {
		t.Fatalf("tool result content type = %T", result.Content[0])
	}
	return content.Text
}

type mcpHeaderCapture struct {
	mu        sync.Mutex
	transport http.RoundTripper
	sessionID bool
}

func (capture *mcpHeaderCapture) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Header.Get("Mcp-Session-Id") != "" {
		capture.recordSessionID()
	}
	response, err := capture.transport.RoundTrip(request)
	if err == nil && response.Header.Get("Mcp-Session-Id") != "" {
		capture.recordSessionID()
	}
	return response, err
}

func (capture *mcpHeaderCapture) recordSessionID() {
	capture.mu.Lock()
	capture.sessionID = true
	capture.mu.Unlock()
}

func (capture *mcpHeaderCapture) hasSessionID() bool {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.sessionID
}
