package journal

import "testing"

func TestEventKeyUsesExactContractVersion(t *testing.T) {
	event := Event{ProposedEvent: ProposedEvent{
		Type:          "knowledge.changed",
		SchemaVersion: 3,
	}}

	key := event.Key()
	if key.Type != event.Type || key.Version != event.SchemaVersion {
		t.Fatalf("Key() = %#v", key)
	}
}

func TestTrimThroughStopsBeforeExclusivePosition(t *testing.T) {
	events := []Event{
		{Sequence: 4},
		{Sequence: 5},
		{Sequence: 6},
	}

	trimmed := TrimThrough(events, 6)
	if len(trimmed) != 2 || trimmed[1].Sequence != 5 {
		t.Fatalf("TrimThrough() = %#v", trimmed)
	}
}
