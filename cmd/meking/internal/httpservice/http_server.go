package httpservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/memoria-space/meking/project"
	querydomain "github.com/memoria-space/meking/query"
	querygraph "github.com/memoria-space/meking/query/graph"
	queryknowledge "github.com/memoria-space/meking/query/knowledge"
	queryreport "github.com/memoria-space/meking/query/report"
)

const maximumHTTPRequestBytes = 64 << 10

type httpApplication struct {
	configuration httpConfiguration
	dependencies  httpDependencies
}

func newHTTPHandler(
	config httpConfiguration,
	allowedHosts []string,
	dependencies httpDependencies,
) (http.Handler, error) {
	if len(allowedHosts) == 0 {
		return nil, errors.New("HTTP allowed hosts are required")
	}
	if err := validateHTTPDependencies(dependencies); err != nil {
		return nil, err
	}
	application := &httpApplication{configuration: config, dependencies: dependencies}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/zones", application.createZone)
	mux.HandleFunc("GET /api/v1/zones", application.listZones)
	mux.HandleFunc("GET /api/v1/zones/{zone_id}", application.zoneDefinition)
	mux.HandleFunc("GET /api/v1/control/policies", application.controlPolicies)
	mux.HandleFunc("GET /api/v1/control/policies/{action}", application.controlPolicy)
	mux.HandleFunc("PUT /api/v1/control/policies/{action}", application.publishControlPolicy)
	zoneRoutes := []struct {
		pattern string
		handler http.Handler
	}{
		{pattern: "GET /api/v1/zones/{zone_id}/runtime", handler: http.HandlerFunc(application.runtime)},
		{pattern: "GET /api/v1/zones/{zone_id}/documents", handler: http.HandlerFunc(application.documents)},
		{pattern: "POST /api/v1/zones/{zone_id}/documents", handler: http.HandlerFunc(application.submitDocument)},
		{pattern: "GET /api/v1/zones/{zone_id}/control/actions", handler: http.HandlerFunc(application.controlActions)},
		{pattern: "GET /api/v1/zones/{zone_id}/control/actions/{action}", handler: http.HandlerFunc(application.controlAction)},
		{pattern: "POST /api/v1/zones/{zone_id}/control/actions/{action}/invoke", handler: http.HandlerFunc(application.invokeControlAction)},
		{pattern: "GET /api/v1/zones/{zone_id}/journal/events", handler: http.HandlerFunc(application.journalEvents)},
		{pattern: "GET /api/v1/zones/{zone_id}/journal/streams/{id...}", handler: http.HandlerFunc(application.journalStream)},
		{pattern: "GET /api/v1/zones/{zone_id}/community-reports", handler: http.HandlerFunc(application.communityReports)},
		{pattern: "GET /api/v1/zones/{zone_id}/community-reports/{id}", handler: http.HandlerFunc(application.communityReport)},
		{pattern: "GET /api/v1/zones/{zone_id}/graph/entities", handler: http.HandlerFunc(application.entityGraph)},
		{pattern: "GET /api/v1/zones/{zone_id}/graph/entities/{id}/neighbors", handler: http.HandlerFunc(application.entityNeighborhood)},
		{pattern: "GET /api/v1/zones/{zone_id}/graph/communities", handler: http.HandlerFunc(application.communityGraph)},
		{pattern: "GET /api/v1/zones/{zone_id}/knowledge/entities", handler: http.HandlerFunc(application.knowledgeEntities)},
		{pattern: "GET /api/v1/zones/{zone_id}/knowledge/entities/{id}", handler: http.HandlerFunc(application.knowledgeEntity)},
		{pattern: "GET /api/v1/zones/{zone_id}/knowledge/relations", handler: http.HandlerFunc(application.knowledgeRelations)},
		{pattern: "GET /api/v1/zones/{zone_id}/knowledge/relations/{id}", handler: http.HandlerFunc(application.knowledgeRelation)},
		{pattern: "GET /api/v1/zones/{zone_id}/knowledge/claims", handler: http.HandlerFunc(application.knowledgeClaims)},
		{pattern: "POST /api/v1/zones/{zone_id}/query", handler: http.HandlerFunc(application.query)},
		{pattern: "POST /api/v1/zones/{zone_id}/query/stream", handler: http.HandlerFunc(application.queryStream)},
		{pattern: "POST /api/v1/zones/{zone_id}/question-suggestions", handler: http.HandlerFunc(application.questionSuggestions)},
	}
	for _, route := range zoneRoutes {
		handler, err := zoneScopeMiddleware(dependencies.zones, route.handler)
		if err != nil {
			return nil, err
		}
		mux.Handle(route.pattern, handler)
	}
	if dependencies.memoryProtocol != nil {
		mux.Handle("/api/v1/mcp", dependencies.memoryProtocol)
	}
	frontend, err := newFrontendHandler()
	if err != nil {
		return nil, fmt.Errorf("open embedded frontend: %w", err)
	}
	mux.Handle("/", frontend)

	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeHTTPErrorValue(writer, http.StatusForbidden, httpError{
			Code: "cross_origin_request", Message: "cross-origin browser requests are not allowed",
		})
	}))
	handler := protection.Handler(validateHTTPHost(allowedHosts, mux))
	return addHTTPSecurityHeaders(handler), nil
}

func (a *httpApplication) runtime(writer http.ResponseWriter, request *http.Request) {
	zoneID, err := requestZoneID(request)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	view, err := a.dependencies.currentReports(request.Context())
	if err != nil && !errors.Is(err, queryreport.ErrNoReportPublication) {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	config := a.configuration
	capabilities := []string{queryMethodBasic, queryMethodLocal, queryMethodGlobal}
	if config.ReportVectorsEnabled {
		capabilities = append(capabilities, queryMethodDRIFT)
	}
	capabilities = append(
		capabilities,
		"streaming",
		"question_suggestions",
		"community_reports",
		"graph_browse",
		"knowledge_browse",
		"document_upload",
		"control_actions",
		"control_policies",
		"journal_events",
		"journal_streams",
	)
	writeHTTPJSON(writer, http.StatusOK, httpRuntimeResponse{
		ContractVersion:    httpContractVersion,
		ApplicationVersion: project.CurrentApplicationVersion(),
		ZoneID:             zoneID,
		EpochID:            view.EpochID,
		ReportSetID:        view.ReportSetID,
		Ready:              view.EpochID > 0,
		Capabilities:       capabilities,
	})
}

func (a *httpApplication) communityReports(writer http.ResponseWriter, request *http.Request) {
	page, err := positiveHTTPQueryInteger(request, "page", 1)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	pageSize, err := positiveHTTPQueryInteger(request, "page_size", 20)
	if err != nil || pageSize > queryreport.MaximumReportPageSize {
		message := "page_size must be a positive integer no greater than " + strconv.Itoa(queryreport.MaximumReportPageSize)
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: message})
		return
	}
	result, err := a.dependencies.browseReports(
		request.Context(),
		queryreport.ReportPageRequest{Page: page, PageSize: pageSize},
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPReportPage(result))
}

func (a *httpApplication) communityReport(writer http.ResponseWriter, request *http.Request) {
	reportID := strings.TrimSpace(request.PathValue("id"))
	if reportID == "" {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code: "invalid_input", Message: "community report id is required",
		})
		return
	}
	epochID, err := requiredEpochID(request)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	result, err := a.dependencies.readReport(request.Context(), reportID, epochID)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPReportDetail(result))
}

func positiveHTTPQueryInteger(request *http.Request, name string, fallback int) (int, error) {
	raw := request.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func nonNegativeHTTPQueryInteger(request *http.Request, name string, fallback int) (int, error) {
	raw := request.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return value, nil
}

func decodeHTTPJSON[T any](writer http.ResponseWriter, request *http.Request, destination *T) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maximumHTTPRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return errors.New("request body must be one valid JSON object within the size limit")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func writeHTTPJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeHTTPErrorResponse(writer http.ResponseWriter, err error, partialOutput bool) {
	status, value := classifyHTTPError(err)
	value.PartialOutput = value.PartialOutput || partialOutput
	writeHTTPErrorValue(writer, status, value)
}

func writeHTTPErrorValue(writer http.ResponseWriter, status int, value httpError) {
	writeHTTPJSON(writer, status, httpErrorEnvelope{Error: value})
}

func classifyHTTPError(err error) (int, httpError) {
	var failure *querydomain.Failure
	if errors.As(err, &failure) {
		value := httpError{
			Code: string(failure.Category), Message: failure.Diagnostic(), Retryable: failure.Retryable,
			StatusCode: failure.StatusCode, PartialOutput: failure.PartialOutput,
		}
		switch failure.Category {
		case querydomain.FailureInvalidInput:
			return http.StatusBadRequest, value
		case querydomain.FailureProviderRetryable:
			return http.StatusServiceUnavailable, value
		case querydomain.FailureProviderPermanent, querydomain.FailureInvalidModelResponse:
			return http.StatusBadGateway, value
		case querydomain.FailureCancelled:
			return http.StatusRequestTimeout, value
		default:
			return http.StatusInternalServerError, value
		}
	}
	switch {
	case errors.Is(err, queryreport.ErrNoReportPublication):
		return http.StatusConflict, httpError{Code: "no_report_publication", Message: "publish a ReportSet before browsing reports"}
	case errors.Is(err, queryreport.ErrReportNotFound):
		return http.StatusNotFound, httpError{Code: "community_report_not_found", Message: "community report was not found"}
	case errors.Is(err, queryreport.ErrInvalidReportRequest):
		return http.StatusBadRequest, httpError{Code: "invalid_input", Message: "report request is invalid"}
	case errors.Is(err, querygraph.ErrNoStructure):
		return http.StatusConflict, httpError{Code: "no_community_structure", Message: "build a Community Structure before browsing Communities"}
	case errors.Is(err, querygraph.ErrEntityNotFound):
		return http.StatusNotFound, httpError{Code: "graph_entity_not_found", Message: "graph entity was not found"}
	case errors.Is(err, querygraph.ErrCommunityNotFound):
		return http.StatusNotFound, httpError{Code: "graph_community_not_found", Message: "graph community was not found"}
	case errors.Is(err, querygraph.ErrInvalidRequest):
		return http.StatusBadRequest, httpError{Code: "invalid_input", Message: "graph browse request is invalid"}
	case errors.Is(err, queryknowledge.ErrNoPublication):
		return http.StatusConflict, httpError{Code: "no_knowledge_publication", Message: "publish an Epoch before browsing Knowledge"}
	case errors.Is(err, queryknowledge.ErrEntityNotFound):
		return http.StatusNotFound, httpError{Code: "knowledge_entity_not_found", Message: "Knowledge Entity was not found"}
	case errors.Is(err, queryknowledge.ErrRelationNotFound):
		return http.StatusNotFound, httpError{Code: "knowledge_relation_not_found", Message: "Knowledge Relation was not found"}
	case errors.Is(err, queryknowledge.ErrInvalidRequest):
		return http.StatusBadRequest, httpError{Code: "invalid_input", Message: "Knowledge browse request is invalid"}
	case errors.Is(err, querydomain.ErrEpochNotFound):
		return http.StatusNotFound, httpError{Code: "epoch_not_found", Message: "the requested Epoch was not found"}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusRequestTimeout, httpError{Code: "cancelled", Message: "request was cancelled"}
	default:
		return http.StatusInternalServerError, httpError{Code: "internal_failure", Message: "request could not be completed"}
	}
}

func validateHTTPHost(allowedHosts []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedHosts))
	for _, host := range allowedHosts {
		allowed[strings.ToLower(strings.TrimSpace(host))] = struct{}{}
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := allowed[strings.ToLower(request.Host)]; !ok {
			writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
				Code: "invalid_host", Message: "request host is not accepted by this service",
			})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

const httpContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

func addHTTPSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Content-Security-Policy", httpContentSecurityPolicy)
		next.ServeHTTP(writer, request)
	})
}
