package httpservice

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	queryapplication "github.com/memoria-space/meking/query/application"
)

func TestHTTPQuestionSuggestionsMapsHistoryAndFixedIdentities(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	var received queryapplication.SuggestionRequest
	dependencies.suggestQuestions = func(
		_ context.Context,
		request queryapplication.SuggestionRequest,
	) (queryapplication.SuggestionExecution, error) {
		received = request
		return queryapplication.SuggestionExecution{
			Questions: []string{"First?", "Second?"}, EpochID: 8,
			ReportSetID: "reports-1", CommunitySetID: "communities-1",
			CorporaID: "corpus-1",
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/question-suggestions",
		[]byte(`{"history":["Earlier?","Current?"],"count":2}`),
	)
	var result httpSuggestionResponse
	decodeHTTPTestResponse(t, response, &result)
	if response.Code != http.StatusOK ||
		!reflect.DeepEqual(result.Questions, []string{"First?", "Second?"}) ||
		result.EpochID != 8 || result.ReportSetID != "reports-1" ||
		result.CommunitySetID != "communities-1" || result.CorporaID != "corpus-1" {
		t.Fatalf("status/result = %d/%#v", response.Code, result)
	}
	if !reflect.DeepEqual(received, queryapplication.SuggestionRequest{
		History: []string{"Earlier?", "Current?"}, Count: 2,
	}) {
		t.Fatalf("received = %#v", received)
	}
}

func TestHTTPQuestionSuggestionsRejectsInvalidInputBeforeExecution(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	called := false
	dependencies.suggestQuestions = func(
		context.Context,
		queryapplication.SuggestionRequest,
	) (queryapplication.SuggestionExecution, error) {
		called = true
		return queryapplication.SuggestionExecution{}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	for _, body := range []string{
		`{"history":[],"count":2}`,
		`{"history":["Current?"],"count":0}`,
		`{"history":["Current?"],"count":21}`,
		`{"history":[" "],"count":1}`,
	} {
		response := serveHTTPRequest(
			t, handler, http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/question-suggestions", []byte(body),
		)
		var failure httpErrorEnvelope
		decodeHTTPTestResponse(t, response, &failure)
		if response.Code != http.StatusBadRequest || failure.Error.Code != "invalid_input" {
			t.Fatalf("body/status/failure = %s/%d/%#v", body, response.Code, failure)
		}
	}
	if called {
		t.Fatal("Question runner was called for invalid input")
	}
}
