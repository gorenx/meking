package httpservice

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
	"github.com/memoria-space/meking/zone"
)

func TestHTTPReadEndpointsExposeDomainPublications(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	dependencies.currentReports = func(context.Context) (queryreport.View, error) {
		return queryreport.View{EpochID: 9, ReportSetID: "report-set-1"}, nil
	}
	dependencies.browseReports = func(
		_ context.Context, request queryreport.ReportPageRequest,
	) (queryreport.ReportPage, error) {
		if request.Page != 2 || request.PageSize != 5 {
			t.Fatalf("report page = %#v", request)
		}
		return queryreport.ReportPage{
			EpochID:     9,
			ReportSetID: "report-set-1", CommunitySetID: "community-set-1",
			CorporaID: "corpora-1", Page: request.Page, PageSize: request.PageSize, Total: 6,
			Reports: []queryreport.ReportSummary{{
				ID: "report-1", CommunityID: "community-1", Title: "Report", Rank: 8.5,
			}},
		}, nil
	}
	dependencies.readReport = func(
		_ context.Context, reportID string, epochID int64,
	) (queryreport.ReportDetail, error) {
		if reportID != "report-1" || epochID != 9 {
			t.Fatalf("report identity = %q/Epoch %d", reportID, epochID)
		}
		return queryreport.ReportDetail{
			EpochID:     9,
			ReportSetID: "report-set-1", CommunitySetID: "community-set-1",
			CorporaID: "corpora-1", ID: reportID, CommunityID: "community-1",
			Title: "Report", FullContent: "# Report",
			Findings: []queryreport.ReportFinding{{Summary: "Finding"}},
			Sources: queryreport.ReportSources{
				Entities: []querybase.KnowledgeReference{{ID: "entity-1", Version: 3}},
			},
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)

	t.Run("runtime", func(t *testing.T) {
		response := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/runtime", nil)
		var body httpRuntimeResponse
		decodeHTTPTestResponse(t, response, &body)
		if response.Code != http.StatusOK || body.ContractVersion != httpContractVersion ||
			body.ZoneID != string(httpTestZoneID) ||
			body.EpochID != 9 || body.ReportSetID != "report-set-1" || !body.Ready ||
			strings.Join(body.Capabilities, ",") != "basic,local,global,drift,streaming,question_suggestions,community_reports,graph_browse,knowledge_browse,document_upload,control_actions,control_policies,journal_events,journal_streams" {
			t.Fatalf("status/runtime = %d/%#v", response.Code, body)
		}
	})

	t.Run("report page", func(t *testing.T) {
		response := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/community-reports?page=2&page_size=5", nil)
		var body httpReportPage
		decodeHTTPTestResponse(t, response, &body)
		if response.Code != http.StatusOK || body.Total != 6 || len(body.Reports) != 1 ||
			body.EpochID != 9 || body.Reports[0].Title != "Report" {
			t.Fatalf("status/body = %d/%#v", response.Code, body)
		}
	})

	t.Run("report detail", func(t *testing.T) {
		response := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/community-reports/report-1?epoch_id=9", nil)
		var body httpReportDetail
		decodeHTTPTestResponse(t, response, &body)
		if response.Code != http.StatusOK || body.FullContent != "# Report" ||
			body.EpochID != 9 ||
			body.ReportSetID != "report-set-1" || body.CommunityID != "community-1" ||
			len(body.Findings) != 1 || len(body.Sources.Entities) != 1 ||
			body.Sources.Entities[0].Version != 3 || body.Children == nil ||
			body.Sources.Relations == nil || body.Sources.Claims == nil ||
			body.Sources.TextUnitIDs == nil {
			t.Fatalf("status/body = %d/%#v", response.Code, body)
		}
	})
}

func TestHTTPRuntimeOmitsDRIFTWhenReportVectorsAreDisabled(t *testing.T) {
	config := httpTestConfig(t)
	config.ReportVectorsEnabled = false
	dependencies := defaultHTTPDependencies()
	dependencies.currentReports = func(context.Context) (queryreport.View, error) {
		return queryreport.View{EpochID: 3, ReportSetID: "reports"}, nil
	}
	handler := mustHTTPHandler(t, config, dependencies)
	response := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/runtime", nil)
	var body httpRuntimeResponse
	decodeHTTPTestResponse(t, response, &body)
	if response.Code != http.StatusOK ||
		strings.Contains(","+strings.Join(body.Capabilities, ",")+",", ",drift,") {
		t.Fatalf("status/runtime = %d/%#v", response.Code, body)
	}
}

func TestHTTPReportBrowseMapsPublicationAndVisibilityFailures(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	dependencies.browseReports = func(
		context.Context,
		queryreport.ReportPageRequest,
	) (queryreport.ReportPage, error) {
		return queryreport.ReportPage{}, queryreport.ErrNoReportPublication
	}
	dependencies.readReport = func(
		context.Context,
		string,
		int64,
	) (queryreport.ReportDetail, error) {
		return queryreport.ReportDetail{}, queryreport.ErrReportNotFound
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)

	missingPublication := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/community-reports", nil)
	var publicationFailure httpErrorEnvelope
	decodeHTTPTestResponse(t, missingPublication, &publicationFailure)
	if missingPublication.Code != http.StatusConflict ||
		publicationFailure.Error.Code != "no_report_publication" {
		t.Fatalf("missing publication status/failure = %d/%#v", missingPublication.Code, publicationFailure)
	}

	missingReport := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/community-reports/report-1?epoch_id=9", nil)
	var reportFailure httpErrorEnvelope
	decodeHTTPTestResponse(t, missingReport, &reportFailure)
	if missingReport.Code != http.StatusNotFound ||
		reportFailure.Error.Code != "community_report_not_found" {
		t.Fatalf("missing Report status/failure = %d/%#v", missingReport.Code, reportFailure)
	}
}

func TestHTTPReportDetailRequiresEpochID(t *testing.T) {
	handler := mustHTTPHandler(t, httpTestConfig(t), defaultHTTPDependencies())
	response := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/community-reports/report-1", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing Epoch response = %d/%s", response.Code, response.Body.String())
	}
}

func TestHTTPRejectsUntrustedHostAndCrossOriginMutation(t *testing.T) {
	handler := mustHTTPHandler(t, httpTestConfig(t), defaultHTTPDependencies())

	request := httptest.NewRequest(http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/runtime", nil)
	request.Host = "attacker.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("untrusted Host status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/query", bytes.NewReader([]byte(`{}`)))
	request.Host = "example.com"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("cross-origin status/headers = %d/%v", response.Code, response.Header())
	}
}

func TestHTTPHandlerServesFrontendWithoutMaskingUnknownAPI(t *testing.T) {
	handler := mustHTTPHandler(t, httpTestConfig(t), defaultHTTPDependencies())

	request := httptest.NewRequest(http.MethodGet, "/client-route", nil)
	request.Host = "example.com"
	request.Header.Set("Accept", "text/html")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<div id="app"></div>`) ||
		response.Header().Get("Content-Security-Policy") != httpContentSecurityPolicy {
		t.Fatalf("frontend status/headers/body = %d/%v/%q", response.Code, response.Header(), response.Body.String())
	}

	missing := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/missing", nil)
	var failure httpErrorEnvelope
	decodeHTTPTestResponse(t, missing, &failure)
	if missing.Code != http.StatusNotFound || failure.Error.Code != "not_found" {
		t.Fatalf("unknown API status/failure = %d/%#v", missing.Code, failure)
	}
}

func TestHTTPHandlerMountsMemoryProtocol(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	called := false
	dependencies.memoryProtocol = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		if request.URL.Path != "/api/v1/mcp" || request.Method != http.MethodPost {
			t.Fatalf("MCP request = %s %s", request.Method, request.URL.Path)
		}
		writer.WriteHeader(http.StatusAccepted)
	})
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(t, handler, http.MethodPost, "/api/v1/mcp", []byte(`{}`))
	if !called || response.Code != http.StatusAccepted {
		t.Fatalf("MCP route = called %t, status %d", called, response.Code)
	}
}

func httpTestConfig(t *testing.T) httpConfiguration {
	t.Helper()
	return httpConfiguration{ReportVectorsEnabled: true}
}

func mustHTTPHandler(
	t *testing.T,
	config httpConfiguration,
	dependencies httpDependencies,
) http.Handler {
	t.Helper()
	dependencies.zones = zoneResolverFunc(func(_ context.Context, id zone.ID) (zone.Definition, error) {
		return zone.NewRootDefinition(id, "", time.Unix(1, 0).UTC())
	})
	handler, err := newHTTPHandler(config, []string{"example.com"}, dependencies)
	if err != nil {
		t.Fatalf("newHTTPHandler() error = %v", err)
	}
	return handler
}

func serveHTTPRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	target string,
	body []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Host = "example.com"
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeHTTPTestResponse[T any](t *testing.T, response *httptest.ResponseRecorder, destination *T) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}
