package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	epochevents "github.com/memoria-space/meking/epoch/integration"
	journalcore "github.com/memoria-space/meking/journal"
)

type Producer struct {
	publisher journalcore.Publisher
}

var _ journalcore.Producer[epochevents.Body] = (*Producer)(nil)

func NewProducer(
	publisher journalcore.Publisher,
) (*Producer, error) {
	if publisher == nil {
		return nil, errors.New("create Epoch Journal producer: Publisher is required")
	}
	return &Producer{publisher: publisher}, nil
}

func EventContracts() []journalcore.EventContractRegistration {
	return []journalcore.EventContractRegistration{
		journalcore.JSONEventRegistration[epochevents.PublishedV1](),
	}
}

func (p *Producer) Publish(
	ctx context.Context,
	events []journalcore.Envelope[epochevents.Body],
) error {
	proposed := make([]journalcore.ProposedEvent, len(events))
	for index, event := range events {
		if event.Body == nil {
			return fmt.Errorf("publish Epoch event %d: Body is required", index)
		}
		body, err := json.Marshal(event.Body)
		if err != nil {
			return fmt.Errorf("encode Epoch event %d: %w", index, err)
		}
		proposed[index] = journalcore.ProposedEvent{
			EventID:       event.EventID,
			StreamID:      event.StreamID,
			Type:          journalcore.EventType(event.Body.EventType()),
			SchemaVersion: journalcore.SchemaVersion(event.Body.SchemaVersion()),
			OccurredAt:    event.OccurredAt,
			CorrelationID: event.CorrelationID,
			CausationID:   event.CausationID,
			Body:          string(body),
		}
	}
	if err := p.publisher.Publish(ctx, proposed); err != nil {
		return fmt.Errorf("publish Epoch events: %w", err)
	}
	return nil
}
