// Package httpservice serves one Project and its Zones through the versioned HTTP API and
// embedded browser application. It owns HTTP protocol, request mapping,
// listener lifetime, and delivery adapters; domain rules remain in use cases.
package httpservice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/assembly"
)

const (
	shutdownWait = 10 * time.Second
)

// Config identifies the one Project root and TCP listener owned by a Web service
// process. Address is a host or IP without a port; Port must be in 1..65535.
type Config struct {
	// ProjectRoot contains the one Project configuration and shared local resources.
	ProjectRoot string
	// AnalysisCommand selects the local executable used only when immutable
	// Project settings require rich-document or Sentence analysis.
	AnalysisCommand string
	// Address is passed to the TCP listener and defaults to loopback in the command.
	Address string
	// Port is the requested TCP port; zero is invalid rather than an ephemeral port.
	Port int
}

// Run opens one Project's shared services,
// serves until cancellation or a service failure, then drains HTTP before
// releasing every owned resource.
func Run(
	ctx context.Context,
	config Config,
	stdout io.Writer,
	stderr io.Writer,
) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateConfig(config); err != nil {
		return err
	}
	application, err := assembly.Open(ctx, assembly.Config{
		Root:            config.ProjectRoot,
		AnalysisCommand: config.AnalysisCommand,
		Logger:          slog.Default(),
	})
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, application.Close()) }()
	listener, err := openListener(config)
	if err != nil {
		return err
	}
	defer listener.Close()
	allowedHosts, port, err := allowedHosts(config.Address, listener.Addr())
	if err != nil {
		return err
	}
	dependencies := defaultHTTPDependencies()
	dependencies.zones = application.Zones()
	dependencies.zoneCatalog = application.Zones()
	dependencies.currentReports = application.Reports().Current
	dependencies.browseReports = application.Reports().Browse
	dependencies.readReport = application.Reports().Read
	dependencies.submitDocument = application.Documents().SubmitDocument
	dependencies.browseDocuments = application.Documents().BrowseDocuments
	dependencies.listActionStatuses = application.ControlStatuses().List
	dependencies.readActionStatus = application.ControlStatuses().Status
	dependencies.invokeAction = application.ControlActions().InvokeManual
	dependencies.listPolicies = application.ControlPolicies().List
	dependencies.readPolicy = application.ControlPolicies().Policy
	dependencies.publishPolicy = application.ControlPolicies().Publish
	dependencies.readJournalEvents = application.Journal().Entries
	dependencies.readStreamEvents = application.Journal().StreamEventsAfter
	dependencies.graphs = application.Graph()
	dependencies.knowledge = application.Knowledge()
	dependencies.queries = queryRunners{
		run:    application.Queries().Run,
		stream: application.Queries().Stream,
	}
	dependencies.suggestQuestions = application.Queries().Suggest
	toolset, err := application.MCP()
	if err != nil {
		return err
	}
	dependencies.memoryProtocol = mcpserver.NewStreamableHTTPServer(
		toolset.Server(),
		mcpserver.WithStateLess(true),
	)
	handler, err := newHTTPHandler(
		httpConfiguration{
			ReportVectorsEnabled: application.DRIFTEnabled(),
		},
		allowedHosts,
		dependencies,
	)
	if err != nil {
		return err
	}
	writeStartMessages(config, port, stdout, stderr)
	return serveProject(ctx, listener, handler, application)
}

type projectProcess interface {
	Run(context.Context) error
}

func serveProject(
	ctx context.Context,
	listener net.Listener,
	handler http.Handler,
	process projectProcess,
) error {
	serviceContext, cancel := context.WithCancel(ctx)
	defer cancel()
	dispatchResult := make(chan error, 1)
	go func() {
		err := process.Run(serviceContext)
		if err != nil {
			cancel()
		}
		dispatchResult <- err
	}()
	serveErr := serve(serviceContext, listener, handler)
	cancel()
	dispatchErr := <-dispatchResult
	if dispatchErr != nil && errors.Is(serveErr, context.Canceled) {
		serveErr = nil
	}
	return errors.Join(serveErr, dispatchErr)
}

func validateConfig(config Config) error {
	if strings.TrimSpace(config.ProjectRoot) == "" {
		return errors.New("HTTP service Project root is required")
	}
	if strings.TrimSpace(config.Address) == "" {
		return errors.New("HTTP service address is required")
	}
	if config.Port <= 0 || config.Port > 65535 {
		return errors.New("HTTP service port must be between 1 and 65535")
	}
	return nil
}

func openListener(config Config) (net.Listener, error) {
	address := strings.Trim(strings.TrimSpace(config.Address), "[]")
	listener, err := net.Listen("tcp", net.JoinHostPort(address, strconv.Itoa(config.Port)))
	if err != nil {
		return nil, fmt.Errorf("listen for Project Web service: %w", err)
	}
	return listener, nil
}

func serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 2 * time.Minute,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}
	serverStopped := make(chan struct{})
	shutdownDone := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownWait)
			defer cancel()
			shutdownDone <- server.Shutdown(shutdownContext)
		case <-serverStopped:
			shutdownDone <- nil
		}
	}()

	serveErr := server.Serve(listener)
	close(serverStopped)
	if shutdownErr := <-shutdownDone; shutdownErr != nil {
		return fmt.Errorf("shut down Project Web service: %w", shutdownErr)
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve Project Web service: %w", serveErr)
}

func allowedHosts(configuredAddress string, listenerAddress net.Addr) ([]string, int, error) {
	tcpAddress, ok := listenerAddress.(*net.TCPAddr)
	if !ok || tcpAddress.Port <= 0 {
		return nil, 0, errors.New("Project Web listener did not expose a TCP port")
	}
	port := tcpAddress.Port
	hosts := make(map[string]struct{})
	add := func(host string) {
		host = strings.Trim(strings.TrimSpace(host), "[]")
		if host != "" {
			hosts[net.JoinHostPort(host, strconv.Itoa(port))] = struct{}{}
		}
	}
	add(configuredAddress)
	add(tcpAddress.IP.String())
	add("localhost")
	add("127.0.0.1")
	add("::1")

	if ip := net.ParseIP(configuredAddress); ip != nil && ip.IsUnspecified() {
		addresses, err := net.InterfaceAddrs()
		if err != nil {
			return nil, 0, fmt.Errorf("enumerate Web listener addresses: %w", err)
		}
		for _, address := range addresses {
			if network, ok := address.(*net.IPNet); ok {
				add(network.IP.String())
			}
		}
	}

	result := make([]string, 0, len(hosts))
	for host := range hosts {
		result = append(result, host)
	}
	return result, port, nil
}

func writeStartMessages(
	config Config,
	port int,
	stdout io.Writer,
	stderr io.Writer,
) {
	address := strings.Trim(strings.TrimSpace(config.Address), "[]")
	if !isLoopbackAddress(address) {
		_, _ = fmt.Fprintln(
			stderr,
			"警告：Web 服务没有内建认证；非 loopback 监听只能部署在受信任网络或外部保护之后。",
		)
	}
	browserAddress := address
	if ip := net.ParseIP(address); ip != nil && ip.IsUnspecified() {
		if ip.To4() == nil {
			browserAddress = "::1"
		} else {
			browserAddress = "127.0.0.1"
		}
	}
	serviceURL := "http://" + net.JoinHostPort(browserAddress, strconv.Itoa(port))
	_, _ = fmt.Fprintf(
		stdout,
		"Meking Web 服务已启动：%s（project_root=%s）\n",
		serviceURL,
		config.ProjectRoot,
	)
	_, _ = fmt.Fprintf(stdout, "Meking MCP endpoint：%s/api/v1/mcp\n", serviceURL)
}

func isLoopbackAddress(address string) bool {
	address = strings.Trim(strings.TrimSpace(address), "[]")
	if strings.EqualFold(address, "localhost") {
		return true
	}
	ip := net.ParseIP(address)
	return ip != nil && ip.IsLoopback()
}
