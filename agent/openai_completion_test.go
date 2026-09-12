package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAICompletionStreamsDeltas(t *testing.T) {
	var requests atomic.Int32
	var captured struct {
		Stream bool `json:"stream"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeCompletionSSE(t, w,
			`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1,"model":"model","choices":[{"index":0,"delta":{"content":"hel"},"finish_reason":""}]}`,
			`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1,"model":"model","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":"stop"}]}`,
		)
	}))
	defer server.Close()

	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1",
		HTTPClient: server.Client(),
	})
	request := CompletionRequest{Messages: []CompletionMessage{{
		Role: CompletionRoleUser, Content: "question",
	}}}
	var deltas []string
	response, err := adapter.Stream(t.Context(), request, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream(first) error = %v", err)
	}
	if response.Content != "hello" || strings.Join(deltas, ",") != "hel,lo" || !captured.Stream {
		t.Fatalf("response/deltas/request = %#v/%v/%#v", response, deltas, captured)
	}
	if requests.Load() != 1 {
		t.Fatalf("provider requests = %d, want 1", requests.Load())
	}
}

func TestOpenAICompletionStreamStopsOnConsumerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeCompletionSSE(t, w,
			`{"id":"chatcmpl-consumer","object":"chat.completion.chunk","created":1,"model":"model","choices":[{"index":0,"delta":{"content":"first"},"finish_reason":""}]}`,
			`{"id":"chatcmpl-consumer","object":"chat.completion.chunk","created":1,"model":"model","choices":[{"index":0,"delta":{"content":"second"},"finish_reason":"stop"}]}`,
		)
	}))
	defer server.Close()
	want := errors.New("downstream closed")
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1",
		HTTPClient: server.Client(),
	})
	calls := 0
	response, err := adapter.Stream(t.Context(), CompletionRequest{}, func(string) error {
		calls++
		if calls == 2 {
			return want
		}
		return nil
	})
	if !errors.Is(err, want) || response.Content != "firstsecond" || calls != 2 {
		t.Fatalf("response/calls/error = %#v/%d/%v", response, calls, err)
	}
}

func TestOpenAICompletionStreamPropagatesCancellationAfterOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-cancel\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"\"}]}\n\n")
		w.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1",
		HTTPClient: server.Client(),
	})
	ctx, cancel := context.WithCancel(t.Context())
	response, err := adapter.Stream(ctx, CompletionRequest{}, func(string) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || response.Content != "partial" {
		t.Fatalf("response/error = %#v/%v, want partial/context.Canceled", response, err)
	}
}

func writeCompletionSSE(t *testing.T, w http.ResponseWriter, payloads ...string) {
	t.Helper()
	for _, payload := range payloads {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			t.Errorf("write SSE: %v", err)
			return
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

func TestOpenAICompletionMapsConversationAndFirstChoice(t *testing.T) {
	type requestMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type requestBody struct {
		Model    string           `json:"model"`
		Messages []requestMessage `json:"messages"`
	}
	var captured requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"gpt-4.1","choices":[{"index":0,"finish_reason":"stop","logprobs":null,"message":{"role":"assistant","content":"first"}},{"index":1,"finish_reason":"stop","logprobs":null,"message":{"role":"assistant","content":"second"}}]}`))
	}))
	defer server.Close()

	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey:     " test-key ",
		Model:      " gpt-4.1 ",
		BaseURL:    server.URL + "/v1",
		HTTPClient: server.Client(),
	})
	got, err := adapter.Complete(context.Background(), CompletionRequest{Messages: []CompletionMessage{
		{Role: CompletionRoleSystem, Content: "system"},
		{Role: CompletionRoleUser, Content: "extract"},
		{Role: CompletionRoleAssistant, Content: "prior"},
		{Role: CompletionRoleUser, Content: "continue"},
	}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got.Content != "first" {
		t.Fatalf("Complete() content = %q, want first", got.Content)
	}
	if captured.Model != "gpt-4.1" {
		t.Errorf("model = %q", captured.Model)
	}
	wantMessages := []requestMessage{{Role: "system", Content: "system"}, {Role: "user", Content: "extract"}, {Role: "assistant", Content: "prior"}, {Role: "user", Content: "continue"}}
	if len(captured.Messages) != len(wantMessages) {
		t.Fatalf("messages = %#v", captured.Messages)
	}
	for index := range wantMessages {
		if captured.Messages[index] != wantMessages[index] {
			t.Errorf("message[%d] = %#v, want %#v", index, captured.Messages[index], wantMessages[index])
		}
	}
}

func TestNewOpenAICompletionValidatesConfig(t *testing.T) {
	tests := []struct {
		name   string
		config OpenAICompletionConfig
		match  string
	}{
		{name: "api key", config: OpenAICompletionConfig{Model: "gpt-4.1"}, match: "API key is required"},
		{name: "unresolved API key", config: OpenAICompletionConfig{APIKey: "${MEKING_API_KEY}", Model: "gpt-4.1"}, match: "not resolved"},
		{name: "model", config: OpenAICompletionConfig{APIKey: "key"}, match: "completion model is required"},
		{
			name: "structured output",
			config: OpenAICompletionConfig{
				APIKey: "key", Model: "model", StructuredOutput: "xml",
			},
			match: "structured output mode",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewOpenAICompletion(test.config)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("NewOpenAICompletion() error = %v, want %q", err, test.match)
			}
		})
	}
}

func TestOpenAICompletionRejectsUnsupportedRoleBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	_, err := adapter.Complete(context.Background(), CompletionRequest{Messages: []CompletionMessage{{Role: "tool", Content: "text"}}})
	if err == nil || !strings.Contains(err.Error(), `unsupported role "tool"`) {
		t.Fatalf("Complete() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestOpenAICompletionMapsJSONObjectResponseFormat(t *testing.T) {
	type responseFormat struct {
		Type string `json:"type"`
	}
	var captured struct {
		ResponseFormat responseFormat `json:"response_format"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"points\":[]}"}}]}`))
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	_, err := adapter.Complete(t.Context(), CompletionRequest{
		ResponseFormat: CompletionResponseJSONObject,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if captured.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v", captured.ResponseFormat)
	}
}

func TestOpenAICompletionTransportsConsumerJSONSchema(t *testing.T) {
	var captured struct {
		ResponseFormat struct {
			Type       string `json:"type"`
			JSONSchema struct {
				Name   string         `json:"name"`
				Strict bool           `json:"strict"`
				Schema map[string]any `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"answer\":\"ok\"}"}}]}`))
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	response, err := adapter.Complete(t.Context(), CompletionRequest{
		Messages:       []CompletionMessage{{Role: CompletionRoleUser, Content: "question"}},
		ResponseFormat: CompletionResponseJSONSchema,
		JSONSchema: &JSONSchema{
			Name:       "answer",
			Definition: json.RawMessage(`{"type":"object","required":["answer"]}`),
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Content != `{"answer":"ok"}` {
		t.Fatalf("content = %q", response.Content)
	}
	if captured.ResponseFormat.Type != "json_schema" ||
		captured.ResponseFormat.JSONSchema.Name != "answer" ||
		!captured.ResponseFormat.JSONSchema.Strict ||
		captured.ResponseFormat.JSONSchema.Schema["type"] != "object" {
		t.Fatalf("response format = %#v", captured.ResponseFormat)
	}
}

func TestOpenAICompletionRejectsInvalidConsumerJSONSchemaBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	_, err := adapter.Complete(t.Context(), CompletionRequest{
		ResponseFormat: CompletionResponseJSONSchema,
		JSONSchema:     &JSONSchema{Name: "answer", Definition: json.RawMessage(`{`)},
	})
	if err == nil || !strings.Contains(err.Error(), "completion JSON schema is invalid JSON") {
		t.Fatalf("Complete() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestOpenAICompletionMapsOptionalMaxCompletionTokens(t *testing.T) {
	var captured struct {
		MaxCompletionTokens int `json:"max_completion_tokens"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	_, err := adapter.Complete(t.Context(), CompletionRequest{MaxCompletionTokens: 321})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if captured.MaxCompletionTokens != 321 {
		t.Fatalf("max_completion_tokens = %d", captured.MaxCompletionTokens)
	}
}

func TestOpenAICompletionRejectsNegativeMaxCompletionTokensBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	_, err := adapter.Complete(t.Context(), CompletionRequest{MaxCompletionTokens: -1})
	if err == nil || !strings.Contains(err.Error(), "must be non-negative") {
		t.Fatalf("Complete() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestOpenAICompletionRejectsUnsupportedResponseFormatBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	_, err := adapter.Complete(t.Context(), CompletionRequest{
		ResponseFormat: "yaml",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("Complete() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestOpenAICompletionKeepsSDKRetriesDisabled(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, `{"error":{"message":"temporary","type":"server_error"}}`, http.StatusInternalServerError)
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{
		APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	})
	_, err := adapter.Complete(context.Background(), CompletionRequest{})
	if err == nil || !strings.Contains(err.Error(), "OpenAI chat completion") {
		t.Fatalf("Complete() error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want SDK retries disabled", requests.Load())
	}
}

func TestOpenAICompletionRejectsResponseWithoutChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"model","choices":[]}`))
	}))
	defer server.Close()
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{APIKey: "key", Model: "model", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	_, err := adapter.Complete(context.Background(), CompletionRequest{})
	if err == nil || !strings.Contains(err.Error(), "returned no choices") {
		t.Fatalf("Complete() error = %v", err)
	}
}

func TestOpenAICompletionPropagatesCancellation(t *testing.T) {
	adapter := mustOpenAICompletion(t, OpenAICompletionConfig{APIKey: "key", Model: "model"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := adapter.Complete(ctx, CompletionRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete() error = %v, want context.Canceled", err)
	}
}

func mustOpenAICompletion(t *testing.T, config OpenAICompletionConfig) *OpenAICompletion {
	t.Helper()
	adapter, err := NewOpenAICompletion(config)
	if err != nil {
		t.Fatalf("NewOpenAICompletion() error = %v", err)
	}
	return adapter
}
