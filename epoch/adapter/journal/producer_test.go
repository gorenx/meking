package journal

import (
	"context"
	"testing"
	"time"

	epochevents "github.com/memoria-space/meking/epoch/integration"
	journalcore "github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
)

func TestProducerLeavesZoneEnvelopeOwnershipToPublisher(t *testing.T) {
	publisher := &recordingPublisher{}
	producer, err := NewProducer(publisher)
	if err != nil {
		t.Fatal(err)
	}
	occurredAt := time.Date(2026, 7, 31, 21, 0, 0, 0, time.UTC)
	body := epochevents.PublishedV1{
		EpochID:   7,
		CorporaID: "11111111-1111-4111-8111-111111111111",
		Knowledge: knowledge.Manifest{
			Entities: []knowledge.Reference[knowledge.EntityID]{
				{ID: "44444444-4444-4444-8444-444444444444", Version: 13},
			},
		},
		StructureID: "22222222-2222-4222-8222-222222222222",
	}
	err = producer.Publish(t.Context(), []journalcore.Envelope[epochevents.Body]{
		{
			EventID: "epoch-published-one", StreamID: "epoch-publication/request-one",
			CorrelationID: "request-one", CausationID: "community-published-one",
			OccurredAt: occurredAt, Body: body,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %#v", publisher.events)
	}
	event := publisher.events[0]
	if event.EventID != "epoch-published-one" || event.ZoneID != "" ||
		event.StreamID != "epoch-publication/request-one" || event.Type != "epoch.published" ||
		event.SchemaVersion != 1 || event.OccurredAt != occurredAt ||
		event.CorrelationID != "request-one" || event.CausationID != "community-published-one" {
		t.Fatalf("published event = %#v", event)
	}
}

type recordingPublisher struct {
	events []journalcore.ProposedEvent
}

func (p *recordingPublisher) Publish(_ context.Context, events []journalcore.ProposedEvent) error {
	p.events = append(p.events, events...)
	return nil
}
