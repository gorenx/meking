package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	journalcore "github.com/memoria-space/meking/journal"
)

type Producer struct {
	publisher journalcore.Publisher
}

var _ corpus.Producer = (*Producer)(nil)

func NewProducer(
	publisher journalcore.Publisher,
) (*Producer, error) {
	if publisher == nil {
		return nil, errors.New("create Corpus Journal producer: Publisher is required")
	}
	return &Producer{publisher: publisher}, nil
}

func EventContracts() []journalcore.EventContractRegistration {
	return []journalcore.EventContractRegistration{
		journalcore.JSONEventRegistration[corpusevents.DocumentRecordedV1](),
		journalcore.JSONEventRegistration[corpusevents.TextPreparedV1](),
		journalcore.JSONEventRegistration[corpusevents.TextUnitsPreparedV1](),
		journalcore.JSONEventRegistration[corpusevents.CorporaPreparedV1](),
	}
}

func (p *Producer) Publish(
	ctx context.Context,
	events []corpus.Event,
) error {
	proposed := make([]journalcore.ProposedEvent, len(events))
	for index, event := range events {
		if event.Body == nil {
			return fmt.Errorf("publish Corpus event %d: Body is required", index)
		}
		body, err := json.Marshal(event.Body)
		if err != nil {
			return fmt.Errorf("encode Corpus event %d: %w", index, err)
		}
		proposed[index] = journalcore.ProposedEvent{
			EventID:       journalcore.EventID(event.EventID),
			StreamID:      journalcore.StreamID(event.StreamID),
			Type:          journalcore.EventType(event.Body.EventType()),
			SchemaVersion: journalcore.SchemaVersion(event.Body.SchemaVersion()),
			OccurredAt:    event.OccurredAt,
			CorrelationID: journalcore.CorrelationID(event.CorrelationID),
			CausationID:   journalcore.EventID(event.CausationID),
			Body:          string(body),
		}
	}
	if err := p.publisher.Publish(ctx, proposed); err != nil {
		return fmt.Errorf("publish Corpus events: %w", err)
	}
	return nil
}
