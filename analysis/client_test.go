package analysis

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientHealthAuthenticatesAndValidatesRequiredCapabilities(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/health" || request.Method != http.MethodGet {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(`{"error":{"code":"unauthorized","message":"denied","retryable":false}}`))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"contract_version":2,
			"api_version":"v1",
			"capabilities":["sentences"],
			"resources":["punkt","punkt_tab"]
		}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL, BearerToken: "test-token", HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	health, err := client.Health(t.Context(), CapabilitySentences)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.ContractVersion != ContractVersion || len(health.Capabilities) != 1 ||
		health.Capabilities[0] != CapabilitySentences {
		t.Fatalf("Health() = %#v", health)
	}
}

func TestClientHealthRejectsUnsafeEndpointAndProtocolDrift(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{
		"https://127.0.0.1:8080", "http://example.com:8080", "http://127.0.0.1:8080/path",
	} {
		if _, err := NewClient(ClientConfig{BaseURL: endpoint, BearerToken: "token"}); err == nil {
			t.Fatalf("NewClient(%q) expected error", endpoint)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{
			"contract_version":1,"api_version":"v1","capabilities":[],"resources":[]
		}`))
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL, BearerToken: "token", HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Health(t.Context())
	var failure *Failure
	if !errors.As(err, &failure) || failure.Kind != FailureUnavailable ||
		strings.Contains(err.Error(), "version 1") {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestClientHealthBoundsAndSanitizesRemoteFailure(t *testing.T) {
	t.Parallel()
	t.Run("controlled error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte(`{"error":{"code":"worker overloaded!!","message":"secret traceback","retryable":true}}`))
		}))
		defer server.Close()
		client, err := NewClient(ClientConfig{
			BaseURL: server.URL, BearerToken: "token", HTTPClient: server.Client(),
		})
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		_, err = client.Health(t.Context())
		var failure *Failure
		if !errors.As(err, &failure) || failure.Kind != FailureFailed || !failure.Retryable ||
			strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "traceback") {
			t.Fatalf("Health() error = %v", err)
		}
	})

	t.Run("oversized body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(strings.Repeat("x", 128)))
		}))
		defer server.Close()
		client, err := NewClient(ClientConfig{
			BaseURL: server.URL, BearerToken: "token", HTTPClient: server.Client(), MaxResponseBytes: 32,
		})
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		_, err = client.Health(t.Context())
		var failure *Failure
		if !errors.As(err, &failure) || failure.Kind != FailureUnavailable {
			t.Fatalf("Health() error = %v", err)
		}
	})
}

func TestClientHealthPropagatesCancellationAsSafeUnavailableFailure(t *testing.T) {
	t.Parallel()
	client, err := NewClient(ClientConfig{BaseURL: "http://127.0.0.1:1", BearerToken: "token"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = client.Health(ctx)
	var failure *Failure
	if !errors.As(err, &failure) || failure.Kind != FailureUnavailable || !failure.Retryable ||
		!errors.Is(err, context.Canceled) {
		t.Fatalf("Health() error = %v", err)
	}
}
