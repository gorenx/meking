package httpservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	querybase "github.com/memoria-space/meking/query"
	queryapplication "github.com/memoria-space/meking/query/application"
)

// httpQueryOutcome is the transport-neutral Query result mapped into HTTP JSON
// or an SSE final event after one selected application completes.
type httpQueryOutcome struct {
	// response is the final model answer after the use case completes.
	response string
	// epochID identifies the unified publication fixed by this execution.
	epochID int64
	// reportSetID and communitySetID identify the fixed report publication when used.
	reportSetID    string
	communitySetID string
	// corporaID identifies the immutable evidence collection fixed by Epoch.
	corporaID string
	// citations contains the audit produced from the fixed evidence scope.
	citations *httpCitationAudit
}

type httpStreamDelta struct {
	Text string `json:"text"`
}

type httpStreamPing struct {
	Time time.Time `json:"time"`
}

type httpStreamResult struct {
	outcome httpQueryOutcome
	err     error
}

func (a *httpApplication) query(writer http.ResponseWriter, request *http.Request) {
	config := a.configuration
	var input httpQueryRequest
	if err := decodeHTTPJSON(writer, request, &input); err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	if err := validateHTTPQueryRequest(input, config.ReportVectorsEnabled); err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	outcome, err := executeHTTPQuery(request.Context(), a.dependencies.queries, input, false, nil)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPQueryResponse(input.Method, outcome))
}

func (a *httpApplication) queryStream(writer http.ResponseWriter, request *http.Request) {
	config := a.configuration
	var input httpQueryRequest
	if err := decodeHTTPJSON(writer, request, &input); err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	if err := validateHTTPQueryRequest(input, config.ReportVectorsEnabled); err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-cache, no-transform")
	writer.WriteHeader(http.StatusOK)
	if err := http.NewResponseController(writer).Flush(); err != nil {
		return
	}

	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	deltas := make(chan string)
	completed := make(chan httpStreamResult, 1)
	go func() {
		outcome, err := executeHTTPQuery(ctx, a.dependencies.queries, input, true, func(delta string) error {
			if delta == "" {
				return nil
			}
			select {
			case deltas <- delta:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		completed <- httpStreamResult{outcome: outcome, err: err}
	}()

	pingInterval := a.dependencies.streamPingInterval
	if pingInterval <= 0 {
		pingInterval = 15 * time.Second
	}
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	partialOutput := false
	for {
		select {
		case delta := <-deltas:
			if err := writeHTTPServerEvent(writer, "delta", httpStreamDelta{Text: delta}); err != nil {
				cancel()
				return
			}
			partialOutput = true
		case result := <-completed:
			if result.err != nil {
				_, value := classifyHTTPError(result.err)
				value.PartialOutput = value.PartialOutput || partialOutput
				_ = writeHTTPServerEvent(writer, "error", value)
				return
			}
			_ = writeHTTPServerEvent(writer, "final", newHTTPQueryResponse(input.Method, result.outcome))
			return
		case now := <-ticker.C:
			if err := writeHTTPServerEvent(writer, "ping", httpStreamPing{Time: now.UTC()}); err != nil {
				cancel()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func validateHTTPQueryRequest(input httpQueryRequest, reportVectorsEnabled bool) error {
	request := newQueryRequest(input)
	if err := queryapplication.ValidateRequest(request); err != nil {
		return err
	}
	if request.Method == queryapplication.MethodDRIFT && !reportVectorsEnabled {
		return querybase.NewInvalidInputFailure(
			"DRIFT is unavailable because report vectors are disabled by Project configuration",
			nil,
		)
	}
	return nil
}

func executeHTTPQuery(
	ctx context.Context,
	runners queryRunners,
	input httpQueryRequest,
	streaming bool,
	emit querybase.TextDeltaHandler,
) (httpQueryOutcome, error) {
	queryRequest := newQueryRequest(input)
	var execution queryapplication.Execution
	var err error
	if streaming {
		execution, err = runners.stream(ctx, queryRequest, emit)
	} else {
		execution, err = runners.run(ctx, queryRequest)
	}
	return httpQueryOutcome{
		response: execution.Response, epochID: execution.EpochID,
		reportSetID: execution.ReportSetID, communitySetID: execution.CommunitySetID,
		corporaID: execution.CorporaID,
		citations: newHTTPCitationAudit(execution.CitationAudit),
	}, err
}

func newQueryRequest(input httpQueryRequest) queryapplication.Request {
	conversation := make([]querybase.ConversationTurn, len(input.Conversation))
	for index, turn := range input.Conversation {
		conversation[index] = querybase.ConversationTurn{
			Role: querybase.ConversationRole(turn.Role), Content: turn.Content,
		}
	}
	var communityLevel *int
	if input.CommunityLevel != nil {
		value := *input.CommunityLevel
		communityLevel = &value
	}
	return queryapplication.Request{
		Method:                    queryapplication.Method(input.Method),
		Question:                  input.Question,
		ResponseType:              input.ResponseType,
		Conversation:              conversation,
		CommunityLevel:            communityLevel,
		DynamicCommunitySelection: input.DynamicCommunitySelection,
		IncludeEntityIDs:          append([]string(nil), input.IncludeEntityIDs...),
		ExcludeEntityIDs:          append([]string(nil), input.ExcludeEntityIDs...),
	}
}

func writeHTTPServerEvent(writer http.ResponseWriter, event string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	return http.NewResponseController(writer).Flush()
}
