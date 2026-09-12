package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProcessSessionStartsAuthenticatesAndStopsHelper(t *testing.T) {
	t.Parallel()
	config := helperProcessConfig("healthy")
	config.RequiredCapabilities = []Capability{CapabilitySentences}
	session, err := Start(t.Context(), config, nil)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if session.Health().Capabilities[0] != CapabilitySentences {
		t.Fatalf("Health() = %#v", session.Health())
	}
	if _, err := session.Client().Health(t.Context(), CapabilitySentences); err != nil {
		t.Fatalf("client Health() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestProcessSessionRejectsMissingCapabilityAndInvalidReady(t *testing.T) {
	t.Parallel()
	t.Run("missing capability", func(t *testing.T) {
		config := helperProcessConfig("healthy")
		config.RequiredCapabilities = []Capability{CapabilityMarkItDown}
		_, err := Start(t.Context(), config, nil)
		var failure *Failure
		if !errors.As(err, &failure) || failure.Kind != FailureUnavailable {
			t.Fatalf("Start() error = %v", err)
		}
	})
	t.Run("invalid ready", func(t *testing.T) {
		_, err := Start(t.Context(), helperProcessConfig("invalid-ready"), nil)
		var failure *Failure
		if !errors.As(err, &failure) || failure.Kind != FailureUnavailable {
			t.Fatalf("Start() error = %v", err)
		}
	})
}

func TestProcessSessionCancellationStopsChildAndBoundsDiagnostics(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	config := helperProcessConfig("noisy")
	config.MaxDiagnosticBytes = 32
	session, err := Start(ctx, config, nil)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if session.process.Signal(syscall.Signal(0)) != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	diagnostics := session.Diagnostics()
	if diagnostics.Bytes < 128 || diagnostics.Retained != 32 || !diagnostics.Truncated {
		t.Fatalf("Diagnostics() = %#v", diagnostics)
	}
}

func TestChildEnvironmentDoesNotInheritOrAcceptModelSecrets(t *testing.T) {
	t.Setenv("MEKING_API_KEY", "parent-secret")
	t.Setenv("OPENAI_API_KEY", "other-secret")
	environment, err := childEnvironment([]string{"NLTK_DATA=/opt/nltk"}, "run-token")
	if err != nil {
		t.Fatalf("childEnvironment() error = %v", err)
	}
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "parent-secret") || strings.Contains(joined, "other-secret") ||
		!strings.Contains(joined, "NLTK_DATA=/opt/nltk") ||
		!strings.Contains(joined, tokenEnvironmentName+"=run-token") {
		t.Fatalf("child environment = %q", joined)
	}
	for _, entry := range []string{
		"MEKING_API_KEY=secret", "SERVICE_TOKEN=secret", "PASSWORD=secret",
		tokenEnvironmentName + "=caller-token",
	} {
		if _, err := childEnvironment([]string{entry}, "run-token"); err == nil {
			t.Fatalf("childEnvironment(%q) expected error", entry)
		}
	}
}

func TestAnalysisHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ANALYSIS_HELPER") != "1" {
		return
	}
	mode := os.Getenv("ANALYSIS_HELPER_MODE")
	token := os.Getenv(tokenEnvironmentName)
	if mode == "invalid-ready" {
		fmt.Println(`{"event":"wrong"}`)
		os.Exit(0)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(2)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/health" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+token {
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(`{"error":{"code":"unauthorized","message":"denied","retryable":false}}`))
			return
		}
		_ = json.NewEncoder(writer).Encode(Health{
			ContractVersion: ContractVersion, APIVersion: APIVersion,
			Capabilities: []Capability{CapabilitySentences}, Resources: []string{"punkt", "punkt_tab"},
		})
	})}
	go func() { _ = server.Serve(listener) }()
	ready, _ := json.Marshal(readyMessage{
		Event: "ready", ContractVersion: ContractVersion, Port: port, PID: os.Getpid(),
		Capabilities: []Capability{CapabilitySentences},
	})
	fmt.Println(string(ready))
	if mode == "noisy" {
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat("diagnostic", 32))
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownContext)
	os.Exit(0)
}

func helperProcessConfig(mode string) ProcessConfig {
	return ProcessConfig{
		Command:        os.Args[0],
		Args:           []string{"-test.run=TestAnalysisHelperProcess", "--", strconv.Quote(mode)},
		Environment:    []string{"GO_WANT_ANALYSIS_HELPER=1", "ANALYSIS_HELPER_MODE=" + mode},
		StartupTimeout: 3 * time.Second, RequestTimeout: time.Second, ShutdownTimeout: time.Second,
	}
}
