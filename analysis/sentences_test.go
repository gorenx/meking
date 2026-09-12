package analysis

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSentencesUsesVersionedAuthenticatedBatch(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/nlp/sentences" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("sentence request did not authenticate")
		}
		var payload sentenceRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.ContractVersion != ContractVersion || len(payload.Documents) != 1 ||
			payload.Documents[0].ID != "document" || payload.Documents[0].Language != "english" {
			t.Fatalf("request = %#v", payload)
		}
		_, _ = writer.Write([]byte(`{
			"contract_version":2,
			"documents":[{"id":"document","spans":[
				{"index":0,"start_char":0,"end_char":10},
				{"index":1,"start_char":12,"end_char":30}
			]}]
		}`))
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL, BearerToken: "test-token", HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	response, err := client.Sentences(t.Context(), []SentenceDocument{{
		ID: "document", Text: "Café works. 再见！ Final sentence.", Language: "english",
	}})
	if err != nil {
		t.Fatalf("Sentences() error = %v", err)
	}
	if len(response.Documents) != 1 || len(response.Documents[0].Spans) != 2 ||
		response.Documents[0].Spans[1].StartChar != 12 {
		t.Fatalf("Sentences() = %#v", response)
	}
}

func TestClientSentencesRejectsInvalidInputAndResponse(t *testing.T) {
	t.Parallel()
	client, err := NewClient(ClientConfig{BaseURL: "http://127.0.0.1:1", BearerToken: "token"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	for _, documents := range [][]SentenceDocument{
		nil,
		{{ID: " document", Text: "Text.", Language: "english"}},
		{{ID: "document", Text: "Text.", Language: "English"}},
		{{ID: "same", Text: "One.", Language: "english"}, {ID: "same", Text: "Two.", Language: "english"}},
	} {
		_, err := client.Sentences(t.Context(), documents)
		var failure *Failure
		if !errors.As(err, &failure) || failure.Kind != FailureFailed || failure.Operation != "sentences_request" {
			t.Fatalf("Sentences(%#v) error = %v", documents, err)
		}
	}

	tests := []struct {
		name string
		body string
	}{
		{name: "version", body: `{"contract_version":1,"documents":[{"id":"document","spans":[]}]}`},
		{name: "alignment", body: `{"contract_version":2,"documents":[{"id":"other","spans":[]}]}`},
		{name: "range", body: `{"contract_version":2,"documents":[{"id":"document","spans":[{"index":0,"start_char":0,"end_char":99}]}]}`},
		{name: "overlap", body: `{"contract_version":2,"documents":[{"id":"document","spans":[{"index":0,"start_char":0,"end_char":2},{"index":1,"start_char":2,"end_char":4}]}]}`},
		{name: "unknown field", body: `{"contract_version":2,"documents":[{"id":"document","spans":[]}],"private":"value"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			remote, err := NewClient(ClientConfig{
				BaseURL: server.URL, BearerToken: "token", HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			_, err = remote.Sentences(t.Context(), []SentenceDocument{{
				ID: "document", Text: "Text.", Language: "english",
			}})
			var failure *Failure
			if !errors.As(err, &failure) || failure.Kind != FailureFailed ||
				failure.Operation != "sentences_response" || strings.Contains(err.Error(), "private") {
				t.Fatalf("Sentences() error = %v", err)
			}
		})
	}
}
