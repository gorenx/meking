package httpservice

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/memoria-space/meking/journal"
)

func TestHTTPJournalEventsReadCommittedOffsetPage(t *testing.T) {
	occurredAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	dependencies := defaultHTTPDependencies()
	dependencies.readJournalEvents = func(
		_ context.Context, offset uint64, limit int,
	) ([]journal.Event, error) {
		if offset != 7 || limit != 3 {
			t.Fatalf("Journal page = Offset %d, limit %d", offset, limit)
		}
		return []journal.Event{
			journalHTTPFixture(8, 3, "event-8", occurredAt),
			journalHTTPFixture(9, 4, "event-9", occurredAt.Add(time.Second)),
			journalHTTPFixture(10, 5, "event-10", occurredAt.Add(2*time.Second)),
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/events?offset=7&limit=2", nil)
	var body httpJournalEventPage
	decodeHTTPTestResponse(t, response, &body)
	if response.Code != http.StatusOK || body.ZoneID != string(httpTestZoneID) ||
		body.Offset != 7 || body.NextOffset != 9 || !body.HasMore || len(body.Events) != 2 {
		t.Fatalf("status/Journal page = %d/%#v", response.Code, body)
	}
	first := body.Events[0]
	if first.Sequence != 8 || first.StreamSequence != 3 || first.EventID != "event-8" ||
		first.Type != "knowledge.published" || first.SchemaVersion != 1 ||
		first.StreamID != "knowledge/formal" || first.CorrelationID != "request-1" ||
		first.CausationID != "event-7" || !first.OccurredAt.Equal(occurredAt) {
		t.Fatalf("first Journal event = %#v", first)
	}
}

func TestHTTPJournalEventsRejectInvalidCursorAndLimit(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	called := false
	dependencies.readJournalEvents = func(context.Context, uint64, int) ([]journal.Event, error) {
		called = true
		return nil, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	for _, path := range []string{
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/events?offset=-1",
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/events?offset=abc",
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/events?offset=18446744073709551615",
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/events?limit=0",
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/events?limit=101",
	} {
		response := serveHTTPRequest(t, handler, http.MethodGet, path, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d/%s", path, response.Code, response.Body.String())
		}
	}
	if called {
		t.Fatal("invalid Journal request reached application dependency")
	}
}

func TestHTTPJournalStreamReadsContiguousSequencePage(t *testing.T) {
	occurredAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	dependencies := defaultHTTPDependencies()
	dependencies.readStreamEvents = func(
		_ context.Context,
		streamID journal.StreamID,
		after journal.StreamSequence,
		limit int,
	) ([]journal.Event, error) {
		if streamID != "knowledge/formal" || after != 5 || limit != 3 {
			t.Fatalf("Journal Stream page = %q after %d, limit %d", streamID, after, limit)
		}
		return []journal.Event{
			journalHTTPFixture(80, 6, "event-80", occurredAt),
			journalHTTPFixture(82, 7, "event-82", occurredAt.Add(time.Second)),
			journalHTTPFixture(85, 8, "event-85", occurredAt.Add(2*time.Second)),
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/streams/knowledge/formal?after_sequence=5&limit=2", nil,
	)
	var body httpJournalStreamPage
	decodeHTTPTestResponse(t, response, &body)
	if response.Code != http.StatusOK || body.ZoneID != string(httpTestZoneID) ||
		body.StreamID != "knowledge/formal" || body.AfterSequence != 5 ||
		body.NextAfterSequence != 7 || !body.HasMore || len(body.Events) != 2 {
		t.Fatalf("status/Journal Stream = %d/%#v", response.Code, body)
	}
	if body.Events[0].StreamSequence != 6 || body.Events[1].StreamSequence != 7 ||
		body.Events[1].Sequence != 82 {
		t.Fatalf("Journal Stream events = %#v", body.Events)
	}
}

func TestHTTPJournalStreamRejectsInvalidIdentityCursorAndLimit(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	called := false
	dependencies.readStreamEvents = func(
		context.Context, journal.StreamID, journal.StreamSequence, int,
	) ([]journal.Event, error) {
		called = true
		return nil, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	for _, path := range []string{
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/streams/%20bad?after_sequence=0",
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/streams/stream-1?after_sequence=-1",
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/streams/stream-1?after_sequence=18446744073709551615",
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/streams/stream-1?limit=0",
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/journal/streams/stream-1?limit=101",
	} {
		response := serveHTTPRequest(t, handler, http.MethodGet, path, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d/%s", path, response.Code, response.Body.String())
		}
	}
	if called {
		t.Fatal("invalid Journal Stream request reached application dependency")
	}
}

func journalHTTPFixture(
	eventSequence journal.EventSequence,
	sequence journal.StreamSequence,
	eventID journal.EventID,
	occurredAt time.Time,
) journal.Event {
	return journal.Event{
		ProposedEvent: journal.ProposedEvent{
			EventID: eventID, ZoneID: journal.ZoneID(httpTestZoneID),
			StreamID: "knowledge/formal", Type: "knowledge.published", SchemaVersion: 1,
			OccurredAt: occurredAt, CorrelationID: "request-1", CausationID: "event-7",
		},
		Sequence: eventSequence, StreamSequence: sequence,
	}
}
