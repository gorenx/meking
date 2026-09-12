package journal

import (
	"testing"
	"time"
)

func TestPendingEventsRejectsPositionPastEventTypeHead(t *testing.T) {
	_, err := validatePendingEvents(PendingEvents{ThroughPosition: 4}, 5)
	if err == nil {
		t.Fatal("validatePendingEvents() succeeded past EventType Position")
	}
}

func TestPendingEventsRequiresSinceForPendingFacts(t *testing.T) {
	_, err := validatePendingEvents(PendingEvents{ThroughPosition: 1, Count: 1}, 0)
	if err == nil {
		t.Fatal("validatePendingEvents() accepted pending facts without Since")
	}
	pending, err := validatePendingEvents(PendingEvents{
		ThroughPosition: 1,
		Count:           1,
		Since:           time.Date(2026, 9, 2, 12, 0, 0, 0, time.FixedZone("test", -7*60*60)),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Since.Location() != time.UTC {
		t.Fatalf("Since location = %v", pending.Since.Location())
	}
}
