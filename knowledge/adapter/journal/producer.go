package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	journalcore "github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge/extraction"
	knowledgeevents "github.com/memoria-space/meking/knowledge/integration"
)

type Producer struct {
	publisher journalcore.Publisher
}

type KnowledgeEvents interface {
	Publish(context.Context, []journalcore.Envelope[knowledgeevents.Body]) error
}

var _ extraction.Producer = (*Producer)(nil)

func NewProducer(
	publisher journalcore.Publisher,
) (*Producer, error) {
	if publisher == nil {
		return nil, errors.New("create Knowledge Journal producer: Publisher is required")
	}
	return &Producer{publisher: publisher}, nil
}

func EventContracts() []journalcore.EventContractRegistration {
	return []journalcore.EventContractRegistration{
		journalcore.JSONEventRegistration[knowledgeevents.PublishedV1](),
		journalcore.JSONEventRegistration[knowledgeevents.EntityVectorsIndexedV1](),
	}
}

func AtomicBatchContracts() []journalcore.AtomicBatchContract {
	return []journalcore.AtomicBatchContract{}
}

func (p *Producer) Publish(
	ctx context.Context,
	events []journalcore.Envelope[knowledgeevents.Body],
) error {
	proposed := make([]journalcore.ProposedEvent, len(events))
	for index, event := range events {
		if event.Body == nil {
			return fmt.Errorf("publish Knowledge event %d: Body is required", index)
		}
		body, err := json.Marshal(event.Body)
		if err != nil {
			return fmt.Errorf("encode Knowledge event %d: %w", index, err)
		}
		proposed[index] = journalcore.ProposedEvent{
			EventID: event.EventID, StreamID: event.StreamID,
			Type:          journalcore.EventType(event.Body.EventType()),
			SchemaVersion: journalcore.SchemaVersion(event.Body.SchemaVersion()),
			OccurredAt:    event.OccurredAt, CorrelationID: event.CorrelationID,
			CausationID: event.CausationID, Body: string(body),
		}
	}
	if err := p.publisher.Publish(ctx, proposed); err != nil {
		return fmt.Errorf("publish Knowledge events: %w", err)
	}
	return nil
}
