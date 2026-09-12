package journal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
	knowledgeevents "github.com/memoria-space/meking/knowledge/integration"
	"github.com/memoria-space/meking/knowledge/vectorindex"
	"github.com/memoria-space/meking/transaction"
)

type EntityVectorIndexing struct {
	transaction.Tx
	Journal    *journalcore.Service
	Positions  journalcore.ConsumerPositions
	Vectors    vectorindex.Builder
	Events     KnowledgeEvents
	ReadLimit  int
}

type EntityVectorAction struct {
	runtime      *actionruntime.Runtime
	transactions transaction.Tx
	vectors      vectorindex.Builder
	events       KnowledgeEvents
}

func NewEntityVectorAction(
	indexing EntityVectorIndexing,
) (*EntityVectorAction, error) {
	switch {
	case indexing.Vectors == nil:
		return nil, errors.New("create Knowledge Entity Vector Action: Vectors are required")
	case indexing.Events == nil:
		return nil, errors.New("create Knowledge Entity Vector Action: Knowledge Events are required")
	}
	action := &EntityVectorAction{
		transactions: indexing.Tx,
		vectors:      indexing.Vectors,
		events:       indexing.Events,
	}
	runtime, err := newActionRuntime(
		"Knowledge Entity Vector",
		"knowledge.entity-vector",
		actionRuntimeDependencies{
			Journal:   indexing.Journal,
			Positions: indexing.Positions,
			ReadLimit: indexing.ReadLimit,
		},
		journalcore.EventKeyFor[knowledgeevents.PublishedV1](),
		action.build,
	)
	if err != nil {
		return nil, err
	}
	action.runtime = runtime
	return action, nil
}

func (action *EntityVectorAction) Pending(ctx context.Context) (actionruntime.PendingInput, error) {
	if action == nil {
		return actionruntime.PendingInput{}, errors.New("read pending Knowledge Entity Vector input: Action is required")
	}
	return action.runtime.Pending(ctx)
}

func (action *EntityVectorAction) Start(ctx context.Context) error {
	if action == nil {
		return errors.New("start Knowledge Entity Vector Action: Action is required")
	}
	return action.runtime.Start(ctx)
}

func (action *EntityVectorAction) Retry(ctx context.Context) error {
	if action == nil {
		return errors.New("retry Knowledge Entity Vector Action: Action is required")
	}
	return action.runtime.Retry(ctx)
}

func (action *EntityVectorAction) Run(ctx context.Context) error {
	if action == nil {
		return errors.New("run Knowledge Entity Vector Action: Action is required")
	}
	return action.runtime.Run(ctx)
}

func (action *EntityVectorAction) build(ctx context.Context, event journalcore.Event) error {
	publication, err := journalcore.DecodeJSONBody[knowledgeevents.PublishedV1](event)
	if err != nil {
		return fmt.Errorf("decode published Knowledge for Entity vectors: %w", err)
	}
	count, err := action.vectors.Build(ctx, vectorindex.Target{
		Entities: publication.Knowledge.Entities,
	})
	if err != nil {
		return fmt.Errorf("build published Knowledge Entity vectors: %w", err)
	}
	if count != len(publication.Knowledge.Entities) {
		return fmt.Errorf(
			"build published Knowledge Entity vectors: indexed %d Entities, expected %d",
			count,
			len(publication.Knowledge.Entities),
		)
	}
	namespace, err := vectorindex.Namespace(publication.Knowledge.Entities)
	if err != nil {
		return err
	}
	body := knowledgeevents.EntityVectorsIndexedV1{
		CorporaID: publication.CorporaID,
		Knowledge: publication.Knowledge,
		Namespace: namespace,
	}
	if err := body.Validate(); err != nil {
		return err
	}
	return action.transactions.WithTx(ctx, func(ctx context.Context) error {
		return action.events.Publish(ctx, []journalcore.Envelope[knowledgeevents.Body]{
			{
				EventID:       entityVectorsEventID(event.EventID),
				StreamID:      event.StreamID,
				CorrelationID: event.CorrelationID,
				CausationID:   event.EventID,
				OccurredAt:    time.Now().UTC(),
				Body:          body,
			},
		})
	})
}

func entityVectorsEventID(source journalcore.EventID) journalcore.EventID {
	digest := sha256.Sum256([]byte(string(source) + "\x00knowledge.entity_vectors_indexed"))
	return journalcore.EventID("event/" + hex.EncodeToString(digest[:]))
}
