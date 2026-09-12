package journal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/community"
	communityevents "github.com/memoria-space/meking/community/integration"
	"github.com/memoria-space/meking/community/structurebuild"
	journalcore "github.com/memoria-space/meking/journal"
)

type Producer struct {
	publisher journalcore.Publisher
}

func (p *Producer) PublishStructure(
	ctx context.Context,
	prepared structurebuild.Prepared,
) error {
	structure, err := community.RestoreStructure(prepared.Structure)
	if err != nil {
		return err
	}
	body := communityevents.StructurePreparedV1{
		StructureID:    string(structure.ID),
		CommunitySetID: communityevents.CommunitySetID(structure.CommunitySetID),
		CorporaID:      communityevents.CorporaID(structure.CorporaID),
		Knowledge:      structure.Knowledge,
	}
	if err := body.Validate(); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(
		prepared.SourceEventID + "\x00" + body.EventType(),
	))
	return p.Publish(ctx, []journalcore.Envelope[communityevents.Body]{
		{
			EventID:       journalcore.EventID("event/" + hex.EncodeToString(digest[:])),
			StreamID:      journalcore.StreamID(communityevents.StructureStream(string(structure.ID))),
			CorrelationID: journalcore.CorrelationID(prepared.CorrelationID),
			CausationID:   journalcore.EventID(prepared.SourceEventID),
			OccurredAt:    structure.CreatedAt,
			Body:          body,
		},
	})
}

var _ journalcore.Producer[communityevents.Body] = (*Producer)(nil)

func NewProducer(
	publisher journalcore.Publisher,
) (*Producer, error) {
	if publisher == nil {
		return nil, errors.New("create Community Journal producer: Publisher is required")
	}
	return &Producer{publisher: publisher}, nil
}

func EventContracts() []journalcore.EventContractRegistration {
	return []journalcore.EventContractRegistration{
		journalcore.JSONEventRegistration[communityevents.StructurePreparedV1](),
	}
}

func (p *Producer) Publish(
	ctx context.Context,
	events []journalcore.Envelope[communityevents.Body],
) error {
	proposed := make([]journalcore.ProposedEvent, len(events))
	for index, event := range events {
		if event.Body == nil {
			return fmt.Errorf("publish Community event %d: Body is required", index)
		}
		body, err := json.Marshal(event.Body)
		if err != nil {
			return fmt.Errorf("encode Community event %d: %w", index, err)
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
		return fmt.Errorf("publish Community events: %w", err)
	}
	return nil
}
