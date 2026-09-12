package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/extraction"
)

type failingCorporaExtractor struct {
	err error
}

func (extractor failingCorporaExtractor) ExtractCorpora(
	context.Context,
	extraction.Request,
) (knowledge.Manifest, error) {
	return knowledge.Manifest{}, extractor.err
}

func TestExtractionActionLeavesRejectedAgentResultIncomplete(t *testing.T) {
	for _, resultError := range []error{
		fmtError(extraction.ErrAgentRequest),
		fmtError(extraction.ErrInvalidGraphExtraction),
		fmtError(extraction.ErrInvalidClaimExtraction),
	} {
		var logs bytes.Buffer
		action := &ExtractionAction{
			extraction: failingCorporaExtractor{err: resultError},
			logger:     slog.New(slog.NewJSONHandler(&logs, nil)),
		}
		event := extractionEvent(t)
		err := action.startExtraction(t.Context(), event)
		if !errors.Is(err, actionruntime.ErrEventIncomplete) ||
			!strings.Contains(err.Error(), "rejected response") {
			t.Fatalf("startExtraction() error = %v", err)
		}
		for _, expected := range []string{
			`"msg":"Knowledge Extraction rejected Agent result"`,
			`"zone_id":"ff8608d5-7e16-4d0e-a795-6e7535a451fa"`,
			`"corpora_id":"1"`,
			`"event_id":"event/1"`,
			`rejected response`,
		} {
			if !strings.Contains(logs.String(), expected) {
				t.Errorf("log = %s, want %s", logs.String(), expected)
			}
		}
	}
}

func TestExtractionActionReturnsNonAgentFailure(t *testing.T) {
	want := errors.New("database unavailable")
	action := &ExtractionAction{
		extraction: failingCorporaExtractor{err: want},
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	err := action.startExtraction(t.Context(), extractionEvent(t))
	if !errors.Is(err, want) {
		t.Fatalf("startExtraction() error = %v, want %v", err, want)
	}
}

func extractionEvent(t *testing.T) journalcore.Event {
	t.Helper()
	eventBody := corpusevents.CorporaPreparedV1{
		CorporaID:         "1",
		TextCount:         1,
		TextUnitSetDigest: strings.Repeat("0", 64),
		TextUnitCount:     1,
		TextUnitSpanCount: 1,
	}
	body, err := json.Marshal(eventBody)
	if err != nil {
		t.Fatal(err)
	}
	return journalcore.Event{ProposedEvent: journalcore.ProposedEvent{
		EventID:       "event/1",
		ZoneID:        "ff8608d5-7e16-4d0e-a795-6e7535a451fa",
		StreamID:      journalcore.StreamID(corpusevents.CorporaStream("1")),
		Type:          journalcore.EventType(eventBody.EventType()),
		SchemaVersion: journalcore.SchemaVersion(eventBody.SchemaVersion()),
		CorrelationID: "correlation/1",
		OccurredAt:    time.Now().UTC(),
		Body:          string(body),
	}}
}

func fmtError(sentinel error) error {
	return errors.Join(sentinel, errors.New("rejected response"))
}
