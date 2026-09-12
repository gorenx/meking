package journal

import (
	"context"
	"errors"
	"fmt"

	communityevents "github.com/memoria-space/meking/community/integration"
	"github.com/memoria-space/meking/epoch"
	"github.com/memoria-space/meking/epoch/lifecycle"
	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
	knowledgeevents "github.com/memoria-space/meking/knowledge/integration"
)

type Publisher interface {
	Publish(context.Context, lifecycle.PublishInput) (epoch.Epoch, error)
}

type PublicationActionDependencies struct {
	Journal    *journalcore.Service
	Positions  journalcore.ConsumerPositions
	Publisher  Publisher
	ReadLimit  int
}

type PublicationAction struct {
	runtime   *actionruntime.Runtime
	journal   *journalcore.Service
	publisher Publisher
	readLimit int
}

type publicationBoundary struct {
	CorporaID string
	Knowledge knowledge.Manifest
}

func NewPublicationAction(
	dependencies PublicationActionDependencies,
) (*PublicationAction, error) {
	switch {
	case dependencies.Publisher == nil:
		return nil, errors.New("create Epoch Publication Action: Publisher is required")
	}
	action := &PublicationAction{
		journal:   dependencies.Journal,
		publisher: dependencies.Publisher,
		readLimit: dependencies.ReadLimit,
	}
	runtime, err := actionruntime.New(actionruntime.Dependencies{
		Name:       "Epoch Publication",
		ConsumerID: "epoch.publication",
		Journal:    dependencies.Journal,
		Positions:  dependencies.Positions,
		EventKeys: []journalcore.EventKey{
			journalcore.EventKeyFor[communityevents.StructurePreparedV1](),
			journalcore.EventKeyFor[knowledgeevents.EntityVectorsIndexedV1](),
		},
		Handle:    action.publishReadyEpoch,
		ReadLimit: dependencies.ReadLimit,
	})
	if err != nil {
		return nil, err
	}
	action.runtime = runtime
	return action, nil
}

func (action *PublicationAction) Pending(ctx context.Context) (actionruntime.PendingInput, error) {
	if action == nil {
		return actionruntime.PendingInput{}, errors.New("read pending Epoch Publication input: Action is required")
	}
	return action.runtime.Pending(ctx)
}

func (action *PublicationAction) Start(ctx context.Context) error {
	if action == nil {
		return errors.New("start Epoch Publication: Action is required")
	}
	return action.runtime.Start(ctx)
}

func (action *PublicationAction) Retry(ctx context.Context) error {
	if action == nil {
		return errors.New("retry Epoch Publication: Action is required")
	}
	return action.runtime.Retry(ctx)
}

func (action *PublicationAction) Run(ctx context.Context) error {
	if action == nil {
		return errors.New("run Epoch Publication Action: Action is required")
	}
	return action.runtime.Run(ctx)
}

func (action *PublicationAction) publishReadyEpoch(
	ctx context.Context,
	event journalcore.Event,
) error {
	switch event.Key() {
	case journalcore.EventKeyFor[communityevents.StructurePreparedV1]():
		return action.publishFromStructure(ctx, event)
	case journalcore.EventKeyFor[knowledgeevents.EntityVectorsIndexedV1]():
		return action.publishFromEntityVectors(ctx, event)
	default:
		return nil
	}
}

func (action *PublicationAction) publishFromStructure(
	ctx context.Context,
	event journalcore.Event,
) error {
	if err := communityevents.ValidateStructureStream(string(event.StreamID)); err != nil {
		return err
	}
	body, err := journalcore.DecodeJSONBody[communityevents.StructurePreparedV1](event)
	if err != nil {
		return fmt.Errorf("decode Community Structure for Epoch Publication: %w", err)
	}
	ready, err := action.hasEntityVectors(ctx, publicationBoundary{
		CorporaID: string(body.CorporaID),
		Knowledge: body.Knowledge,
	})
	if err != nil || !ready {
		return err
	}
	return action.publish(ctx, event, body)
}

func (action *PublicationAction) publishFromEntityVectors(
	ctx context.Context,
	event journalcore.Event,
) error {
	body, err := journalcore.DecodeJSONBody[knowledgeevents.EntityVectorsIndexedV1](event)
	if err != nil {
		return fmt.Errorf("decode Knowledge Entity vectors for Epoch Publication: %w", err)
	}
	structure, found, err := action.findStructure(ctx, publicationBoundary{
		CorporaID: string(body.CorporaID),
		Knowledge: body.Knowledge,
	})
	if err != nil || !found {
		return err
	}
	return action.publish(ctx, event, structure)
}

func (action *PublicationAction) publish(
	ctx context.Context,
	event journalcore.Event,
	body communityevents.StructurePreparedV1,
) error {
	_, err := action.publisher.Publish(ctx, lifecycle.PublishInput{
		CorrelationID: event.CorrelationID,
		CausationID:   event.EventID,
		StructureID:   epoch.StructureID(body.StructureID),
		CorporaID:     epoch.CorporaID(body.CorporaID),
		Knowledge:     body.Knowledge,
	})
	if err != nil {
		return fmt.Errorf("publish Epoch from Community Structure %q: %w", body.StructureID, err)
	}
	return nil
}

func (action *PublicationAction) hasEntityVectors(
	ctx context.Context,
	boundary publicationBoundary,
) (bool, error) {
	position := journalcore.Position(0)
	through, err := action.journal.EventTypePosition(
		ctx,
		journalcore.EventKeyFor[knowledgeevents.EntityVectorsIndexedV1]().Type,
	)
	if err != nil {
		return false, err
	}
	for position < through {
		events, err := action.journal.Events(
			ctx,
			journalcore.EventKeyFor[knowledgeevents.EntityVectorsIndexedV1]().Type,
			position,
			through,
			action.readLimit,
		)
		if err != nil {
			return false, err
		}
		if len(events) == 0 {
			return false, nil
		}
		for _, event := range events {
			if event.Key() != journalcore.EventKeyFor[knowledgeevents.EntityVectorsIndexedV1]() {
				continue
			}
			body, err := journalcore.DecodeJSONBody[knowledgeevents.EntityVectorsIndexedV1](event)
			if err != nil {
				return false, err
			}
			if string(body.CorporaID) == boundary.CorporaID && body.Knowledge.Equal(boundary.Knowledge) {
				return true, nil
			}
		}
		position = journalcore.Position(events[len(events)-1].Sequence) + 1
	}
	return false, nil
}

func (action *PublicationAction) findStructure(
	ctx context.Context,
	boundary publicationBoundary,
) (communityevents.StructurePreparedV1, bool, error) {
	position := journalcore.Position(0)
	through, err := action.journal.EventTypePosition(
		ctx,
		journalcore.EventKeyFor[communityevents.StructurePreparedV1]().Type,
	)
	if err != nil {
		return communityevents.StructurePreparedV1{}, false, err
	}
	for position < through {
		events, err := action.journal.Events(
			ctx,
			journalcore.EventKeyFor[communityevents.StructurePreparedV1]().Type,
			position,
			through,
			action.readLimit,
		)
		if err != nil {
			return communityevents.StructurePreparedV1{}, false, err
		}
		if len(events) == 0 {
			return communityevents.StructurePreparedV1{}, false, nil
		}
		for _, event := range events {
			if event.Key() != journalcore.EventKeyFor[communityevents.StructurePreparedV1]() {
				continue
			}
			body, err := journalcore.DecodeJSONBody[communityevents.StructurePreparedV1](event)
			if err != nil {
				return communityevents.StructurePreparedV1{}, false, err
			}
			if string(body.CorporaID) == boundary.CorporaID && body.Knowledge.Equal(boundary.Knowledge) {
				return body, true, nil
			}
		}
		position = journalcore.Position(events[len(events)-1].Sequence) + 1
	}
	return communityevents.StructurePreparedV1{}, false, nil
}
