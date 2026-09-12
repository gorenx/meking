package analysis

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientNounPhrasesUsesAuthenticatedVersionedBatch(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/nlp/noun-phrases" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("noun phrase request did not authenticate")
		}
		var payload nounPhraseRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.ContractVersion != ContractVersion || len(payload.TextUnits) != 1 ||
			payload.TextUnits[0].ID != "item-0" ||
			payload.Analyzer.ExtractorType != NounPhraseExtractorRegexEnglish {
			t.Fatalf("request = %#v", payload)
		}
		_, _ = writer.Write([]byte(`{
			"contract_version":2,
			"text_units":[{"id":"item-0","phrases":["ALICE SMITH","MICROSOFT"]}]
		}`))
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL, BearerToken: "test-token", HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	response, err := client.NounPhrases(t.Context(), []NounPhraseTextUnit{{
		ID: "item-0", Text: "Microsoft met Alice Smith.",
	}}, regexNounPhraseConfig())
	if err != nil {
		t.Fatalf("NounPhrases() error = %v", err)
	}
	if len(response.TextUnits) != 1 || len(response.TextUnits[0].Phrases) != 2 ||
		response.TextUnits[0].Phrases[1] != "MICROSOFT" {
		t.Fatalf("NounPhrases() = %#v", response)
	}
}

func TestClientNounPhrasesRejectsInvalidInputAndResponse(t *testing.T) {
	t.Parallel()
	client, err := NewClient(ClientConfig{BaseURL: "http://127.0.0.1:1", BearerToken: "token"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	for _, test := range []struct {
		textUnits []NounPhraseTextUnit
		config    NounPhraseAnalyzerConfig
	}{
		{config: regexNounPhraseConfig()},
		{textUnits: []NounPhraseTextUnit{{ID: " same", Text: "text"}}, config: regexNounPhraseConfig()},
		{textUnits: []NounPhraseTextUnit{{ID: "same"}, {ID: "same"}}, config: regexNounPhraseConfig()},
		{textUnits: []NounPhraseTextUnit{{ID: "item"}}, config: NounPhraseAnalyzerConfig{}},
	} {
		_, err := client.NounPhrases(t.Context(), test.textUnits, test.config)
		var failure *Failure
		if !errors.As(err, &failure) || failure.Kind != FailureFailed ||
			failure.Operation != "noun_phrases_request" {
			t.Fatalf("NounPhrases(%#v) error = %v", test, err)
		}
	}

	tests := []struct {
		name string
		body string
	}{
		{name: "version", body: `{"contract_version":1,"text_units":[{"id":"item","phrases":[]}]}`},
		{name: "alignment", body: `{"contract_version":2,"text_units":[{"id":"other","phrases":[]}]}`},
		{name: "unsorted", body: `{"contract_version":2,"text_units":[{"id":"item","phrases":["B","A"]}]}`},
		{name: "duplicate", body: `{"contract_version":2,"text_units":[{"id":"item","phrases":["A","A"]}]}`},
		{name: "noncanonical case", body: `{"contract_version":2,"text_units":[{"id":"item","phrases":["Beta"]}]}`},
		{name: "control", body: `{"contract_version":2,"text_units":[{"id":"item","phrases":["A\nB"]}]}`},
		{name: "unknown", body: `{"contract_version":2,"text_units":[{"id":"item","phrases":[]}],"private":"value"}`},
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
			_, err = remote.NounPhrases(t.Context(), []NounPhraseTextUnit{{ID: "item"}}, regexNounPhraseConfig())
			var failure *Failure
			if !errors.As(err, &failure) || failure.Kind != FailureFailed ||
				failure.Operation != "noun_phrases_response" || strings.Contains(err.Error(), "private") {
				t.Fatalf("NounPhrases() error = %v", err)
			}
		})
	}
}

func regexNounPhraseConfig() NounPhraseAnalyzerConfig {
	return NounPhraseAnalyzerConfig{
		ExtractorType: NounPhraseExtractorRegexEnglish,
		ExcludeNouns:  []string{"THING", "PEOPLE"},
		MaxWordLength: 15,
		WordDelimiter: " ",
	}
}
