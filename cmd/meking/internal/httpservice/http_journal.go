package httpservice

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/memoria-space/meking/journal"
)

const (
	defaultJournalPageSize = 50
	maximumJournalPageSize = 100
)

func (a *httpApplication) journalEvents(writer http.ResponseWriter, request *http.Request) {
	zoneID, err := requestZoneID(request)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	offset, err := journalCursor(request, "offset")
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	limit, err := positiveHTTPQueryInteger(request, "limit", defaultJournalPageSize)
	if err != nil || limit > maximumJournalPageSize {
		message := "limit must be a positive integer no greater than " + strconv.Itoa(maximumJournalPageSize)
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: message})
		return
	}
	events, err := a.dependencies.readJournalEvents(
		request.Context(), offset, limit+1,
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	nextOffset := offset + uint64(len(events))
	writeHTTPJSON(writer, http.StatusOK, httpJournalEventPage{
		ZoneID: zoneID, Offset: offset,
		NextOffset: nextOffset, HasMore: hasMore, Events: newHTTPJournalEvents(events),
	})
}

func (a *httpApplication) journalStream(writer http.ResponseWriter, request *http.Request) {
	zoneID, err := requestZoneID(request)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	streamID := request.PathValue("id")
	if streamID == "" || strings.TrimSpace(streamID) != streamID || len(streamID) > 512 {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code: "invalid_input", Message: "stream id is invalid",
		})
		return
	}
	after, err := journalCursor(request, "after_sequence")
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	limit, err := positiveHTTPQueryInteger(request, "limit", defaultJournalPageSize)
	if err != nil || limit > maximumJournalPageSize {
		message := "limit must be a positive integer no greater than " + strconv.Itoa(maximumJournalPageSize)
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: message})
		return
	}
	events, err := a.dependencies.readStreamEvents(
		request.Context(), journal.StreamID(streamID), journal.StreamSequence(after), limit+1,
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	nextAfter := after
	for _, event := range events {
		nextAfter = uint64(event.StreamSequence)
	}
	writeHTTPJSON(writer, http.StatusOK, httpJournalStreamPage{
		ZoneID: zoneID, StreamID: streamID,
		AfterSequence: after, NextAfterSequence: nextAfter,
		HasMore: hasMore, Events: newHTTPJournalEvents(events),
	})
}

func newHTTPJournalEvents(events []journal.Event) []httpJournalEvent {
	mapped := make([]httpJournalEvent, len(events))
	for index, event := range events {
		mapped[index] = httpJournalEvent{
			Sequence: uint64(event.Sequence), EventID: string(event.EventID),
			ZoneID: string(event.ZoneID),
			Type:   string(event.Type), SchemaVersion: uint32(event.SchemaVersion),
			StreamID: string(event.StreamID), StreamSequence: uint64(event.StreamSequence),
			OccurredAt: event.OccurredAt, CorrelationID: string(event.CorrelationID),
			CausationID: string(event.CausationID),
		}
	}
	return mapped
}

func journalCursor(request *http.Request, name string) (uint64, error) {
	raw := request.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("%s exceeds the supported Journal range", name)
	}
	return value, nil
}
