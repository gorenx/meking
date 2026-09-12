package httpservice

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	querybase "github.com/memoria-space/meking/query"
	queryapplication "github.com/memoria-space/meking/query/application"
	querysource "github.com/memoria-space/meking/query/source"
)

func TestHTTPQueryMapsGlobalReportSetRequestAndCitationResult(t *testing.T) {
	config := httpTestConfig(t)
	dependencies := defaultHTTPDependencies()
	var received queryapplication.Request
	dependencies.queries.run = func(
		_ context.Context,
		input queryapplication.Request,
	) (queryapplication.Execution, error) {
		received = input
		return queryapplication.Execution{
			Method: queryapplication.MethodGlobal, Response: "Answer [Data: Reports (0)]",
			EpochID: 9, ReportSetID: "report-set-1", CommunitySetID: "community-set-1",
			CorporaID: "corpora-1",
			CitationAudit: querybase.CitationAudit{Items: []querybase.Citation{{
				Raw: "Reports (0)", Parsed: true, Status: querybase.CitationValid,
				Reference: querybase.CitationReference{Dataset: querybase.CitationReports, RecordID: 0},
				Sources: []querysource.TextUnitSource{{
					CorporaID: "corpora-1", TextUnitID: "text-1",
					DocumentID:       "document-1",
					DocumentLocation: "input/source.txt", TextTitle: "source.txt",
				}},
			}}},
		}, nil
	}
	handler := mustHTTPHandler(t, config, dependencies)
	response := serveHTTPRequest(
		t,
		handler,
		http.MethodPost,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/query",
		[]byte(`{"method":"global","question":"What?","community_level":1,"dynamic_community_selection":true,"conversation":[{"role":"user","content":"Earlier?"}]}`),
	)
	var result httpQueryResponse
	decodeHTTPTestResponse(t, response, &result)
	if response.Code != http.StatusOK || result.Method != queryMethodGlobal ||
		result.EpochID != 9 || result.ReportSetID != "report-set-1" ||
		result.CommunitySetID != "community-set-1" || result.CorporaID != "corpora-1" ||
		result.CitationAudit == nil ||
		len(result.CitationAudit.Items) != 1 || len(result.CitationAudit.Items[0].Sources) != 1 {
		t.Fatalf("status/result = %d/%#v", response.Code, result)
	}
	source := result.CitationAudit.Items[0].Sources[0]
	if result.CitationAudit.Items[0].RecordID == nil ||
		*result.CitationAudit.Items[0].RecordID != 0 ||
		source.CorporaID != "corpora-1" ||
		source.DocumentLocation != "input/source.txt" {
		t.Fatalf("Global citation source = %#v", source)
	}
	if received.Method != queryapplication.MethodGlobal || received.Question != "What?" ||
		!received.DynamicCommunitySelection ||
		received.CommunityLevel == nil || *received.CommunityLevel != 1 ||
		len(received.Conversation) != 1 {
		t.Fatalf("received Query request = %#v", received)
	}
}

func TestHTTPQueryRejectsMethodSpecificFieldsBeforeExecution(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	called := false
	dependencies.queries.run = func(
		context.Context,
		queryapplication.Request,
	) (queryapplication.Execution, error) {
		called = true
		return queryapplication.Execution{}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/query",
		[]byte(`{"method":"basic","question":"What?","community_level":1}`),
	)
	var failure httpErrorEnvelope
	decodeHTTPTestResponse(t, response, &failure)
	if response.Code != http.StatusBadRequest || failure.Error.Code != "invalid_input" || called {
		t.Fatalf("status/failure/called = %d/%#v/%t", response.Code, failure, called)
	}
}

func TestHTTPQueryMapsEverySupportedMethodToOneServiceContract(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	received := make([]queryapplication.Request, 0, 4)
	dependencies.queries.run = func(
		_ context.Context,
		request queryapplication.Request,
	) (queryapplication.Execution, error) {
		received = append(received, request)
		return queryapplication.Execution{
			Method: request.Method, EpochID: 4,
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	requests := []string{
		`{"method":"basic","question":"basic"}`,
		`{"method":"local","question":"local","include_entity_ids":["entity-a"],"exclude_entity_ids":["entity-b"]}`,
		`{"method":"global","question":"global"}`,
		`{"method":"drift","question":"drift"}`,
	}
	for _, body := range requests {
		response := serveHTTPRequest(t, handler, http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/query", []byte(body))
		if response.Code != http.StatusOK {
			t.Fatalf("query %s status/body = %d/%s", body, response.Code, response.Body.String())
		}
	}
	if len(received) != 4 || received[0].Method != queryapplication.MethodBasic ||
		received[1].Method != queryapplication.MethodLocal ||
		len(received[1].IncludeEntityIDs) != 1 || len(received[1].ExcludeEntityIDs) != 1 ||
		received[2].Method != queryapplication.MethodGlobal || received[3].Method != queryapplication.MethodDRIFT {
		t.Fatalf("received requests = %#v", received)
	}
}

func TestHTTPQueryRejectsDRIFTWhenReportVectorsAreDisabled(t *testing.T) {
	config := httpTestConfig(t)
	config.ReportVectorsEnabled = false
	dependencies := defaultHTTPDependencies()
	called := false
	dependencies.queries.run = func(
		context.Context,
		queryapplication.Request,
	) (queryapplication.Execution, error) {
		called = true
		return queryapplication.Execution{}, nil
	}
	handler := mustHTTPHandler(t, config, dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/query",
		[]byte(`{"method":"drift","question":"What?"}`),
	)
	var failure httpErrorEnvelope
	decodeHTTPTestResponse(t, response, &failure)
	if response.Code != http.StatusBadRequest || failure.Error.Code != "invalid_input" || called {
		t.Fatalf("status/failure/called = %d/%#v/%t", response.Code, failure, called)
	}
}

func TestHTTPQueryStreamEmitsDeltaAndTerminalResult(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	dependencies.queries.stream = func(
		ctx context.Context,
		_ queryapplication.Request,
		emit querybase.TextDeltaHandler,
	) (queryapplication.Execution, error) {
		for _, delta := range []string{"Hel", "lo"} {
			if err := emit(delta); err != nil {
				return queryapplication.Execution{}, err
			}
		}
		return queryapplication.Execution{EpochID: 9, ReportSetID: "report-set-1"}, ctx.Err()
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/query/stream", []byte(`{"method":"global","question":"What?"}`),
	)
	stream := response.Body.String()
	if response.Code != http.StatusOK ||
		response.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" ||
		!strings.Contains(stream, "event: delta\ndata: {\"text\":\"Hel\"}") ||
		!strings.Contains(stream, "event: delta\ndata: {\"text\":\"lo\"}") ||
		!strings.Contains(stream, "event: final") || strings.Contains(stream, "event: error") {
		t.Fatalf("status/stream = %d/%q", response.Code, stream)
	}
}

func TestHTTPQueryStreamMarksFailureAfterPartialOutput(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	dependencies.queries.stream = func(
		_ context.Context,
		_ queryapplication.Request,
		emit querybase.TextDeltaHandler,
	) (queryapplication.Execution, error) {
		if err := emit("partial"); err != nil {
			return queryapplication.Execution{}, err
		}
		return queryapplication.Execution{}, querybase.NewProviderFailure(true, 503, errors.New("secret body"))
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/query/stream", []byte(`{"method":"global","question":"What?"}`),
	)
	stream := response.Body.String()
	if !strings.Contains(stream, "event: delta") || !strings.Contains(stream, "event: error") ||
		!strings.Contains(stream, `"partial_output":true`) || strings.Contains(stream, "secret body") ||
		strings.Contains(stream, "event: final") {
		t.Fatalf("stream = %q", stream)
	}
}

func TestHTTPQueryStreamSendsPingWhileWaiting(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	dependencies.streamPingInterval = time.Millisecond
	dependencies.queries.stream = func(
		context.Context,
		queryapplication.Request,
		querybase.TextDeltaHandler,
	) (queryapplication.Execution, error) {
		time.Sleep(4 * time.Millisecond)
		return queryapplication.Execution{}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/query/stream", []byte(`{"method":"global","question":"What?"}`),
	)
	if stream := response.Body.String(); !strings.Contains(stream, "event: ping") ||
		!strings.Contains(stream, "event: final") {
		t.Fatalf("stream = %q", stream)
	}
}
