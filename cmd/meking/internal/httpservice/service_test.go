package httpservice

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestAllowedHostsUsesActualListenerPort(t *testing.T) {
	t.Parallel()

	hosts, port, err := allowedHosts(
		"127.0.0.1",
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 43210},
	)
	if err != nil {
		t.Fatalf("allowedHosts() error = %v", err)
	}
	if port != 43210 {
		t.Fatalf("port = %d", port)
	}
	for _, expected := range []string{"127.0.0.1:43210", "localhost:43210", "[::1]:43210"} {
		if !slices.Contains(hosts, expected) {
			t.Fatalf("hosts = %v, missing %q", hosts, expected)
		}
	}
}

func TestLoopbackAddress(t *testing.T) {
	t.Parallel()

	for _, address := range []string{"127.0.0.1", "::1", "[::1]", "localhost"} {
		if !isLoopbackAddress(address) {
			t.Errorf("isLoopbackAddress(%q) = false", address)
		}
	}
	for _, address := range []string{"0.0.0.0", "::", "192.0.2.1", "web.example"} {
		if isLoopbackAddress(address) {
			t.Errorf("isLoopbackAddress(%q) = true", address)
		}
	}
}

func TestWriteStartMessagesPrintsMCPEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		address  string
		endpoint string
	}{
		{
			name:     "loopback",
			address:  "127.0.0.1",
			endpoint: "Meking MCP endpoint：http://127.0.0.1:8080/api/v1/mcp",
		},
		{
			name:     "all IPv6 interfaces",
			address:  "::",
			endpoint: "Meking MCP endpoint：http://[::1]:8080/api/v1/mcp",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			writeStartMessages(
				Config{ProjectRoot: "/tmp/meking", Address: test.address},
				8080,
				&stdout,
				&stderr,
			)

			if !strings.Contains(stdout.String(), test.endpoint) {
				t.Fatalf("stdout = %q, missing %q", stdout.String(), test.endpoint)
			}
		})
	}
}

func TestServeCancelsOpenMCPStreamBeforeShutdown(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	serveResult := make(chan error, 1)
	protocol := mcpserver.NewStreamableHTTPServer(
		mcpserver.NewMCPServer("shutdown-test", "1"),
		mcpserver.WithStateLess(true),
	)
	go func() {
		serveResult <- serve(ctx, listener, protocol)
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if contentType := response.Header.Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("MCP GET Content-Type = %q", contentType)
	}

	cancel()

	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP service did not stop with an open MCP stream")
	}
}
