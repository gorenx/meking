package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	epoch "github.com/memoria-space/meking/epoch"
	epochevents "github.com/memoria-space/meking/epoch/integration"
	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/transaction"
)

type Producer interface {
	Publish(context.Context, []journal.Envelope[epochevents.Body]) error
}

type ApplicationDependencies struct {
	Transactions transaction.Tx
	Store        epoch.Store
	Readiness    epoch.Readiness
	Producer     Producer
}

type Application struct {
	transactions transaction.Tx
	store        epoch.Store
	readiness    epoch.Readiness
	producer     Producer
}

type PublishInput struct {
	CorrelationID journal.CorrelationID
	CausationID   journal.EventID
	StructureID   epoch.StructureID
	CorporaID     epoch.CorporaID
	Knowledge     knowledge.Manifest
}

func NewApplication(dependencies ApplicationDependencies) (*Application, error) {
	switch {
	case dependencies.Transactions == nil:
		return nil, errors.New("create Epoch Publication: Transactions are required")
	case dependencies.Store == nil:
		return nil, errors.New("create Epoch Publication: Store is required")
	case dependencies.Readiness == nil:
		return nil, errors.New("create Epoch Publication: Readiness is required")
	case dependencies.Producer == nil:
		return nil, errors.New("create Epoch Publication: Producer is required")
	}
	return &Application{
		transactions: dependencies.Transactions,
		store:        dependencies.Store,
		readiness:    dependencies.Readiness,
		producer:     dependencies.Producer,
	}, nil
}

func (application *Application) Publish(
	ctx context.Context,
	input PublishInput,
) (epoch.Epoch, error) {
	if application == nil {
		return epoch.Epoch{}, errors.New("publish Epoch: Publication is required")
	}
	if strings.TrimSpace(string(input.CorrelationID)) == "" ||
		strings.TrimSpace(string(input.CausationID)) == "" {
		return epoch.Epoch{}, epoch.ErrInvalidEpoch
	}
	target := epoch.PublicationTarget{
		Knowledge:   input.Knowledge,
		StructureID: input.StructureID,
	}
	if err := epoch.ValidatePublicationTarget(target); err != nil {
		return epoch.Epoch{}, err
	}
	existing, err := application.store.LoadStructure(ctx, input.StructureID)
	if err == nil {
		if existing.CorporaID != input.CorporaID ||
			!existing.Knowledge.Equal(input.Knowledge) {
			return epoch.Epoch{}, epoch.ErrEpochConflict
		}
		return existing, nil
	}
	if !errors.Is(err, epoch.ErrEpochNotFound) {
		return epoch.Epoch{}, err
	}
	if err := application.readiness.Check(
		ctx,
		input.StructureID,
		input.CorporaID,
		input.Knowledge,
	); err != nil {
		return epoch.Epoch{}, fmt.Errorf("check Epoch readiness: %w", err)
	}
	current, err := application.store.Current(ctx)
	switch {
	case err == nil:
		target.ExpectedEpoch = current.ID
	case errors.Is(err, epoch.ErrEpochNotFound):
		target.ExpectedEpoch = 0
	default:
		return epoch.Epoch{}, fmt.Errorf("read Current Epoch before publication: %w", err)
	}

	var published epoch.Epoch
	err = application.transactions.WithTx(ctx, func(ctx context.Context) error {
		var err error
		published, err = application.store.Publish(
			ctx,
			target,
			input.CorporaID,
			time.Now().UTC(),
		)
		if err != nil {
			return err
		}
		body := epochevents.PublishedV1{
			EpochID:     epochevents.EpochID(published.ID),
			CorporaID:   epochevents.CorporaID(published.CorporaID),
			Knowledge:   published.Knowledge,
			StructureID: epochevents.StructureID(published.StructureID),
		}
		return application.producer.Publish(ctx, []journal.Envelope[epochevents.Body]{
			{
				EventID:       epochEventID(input.CausationID, body.EventType()),
				StreamID:      journal.StreamID(epochevents.Stream(string(published.StructureID))),
				CorrelationID: input.CorrelationID,
				CausationID:   input.CausationID,
				OccurredAt:    published.PublishedAt,
				Body:          body,
			},
		})
	})
	if err != nil {
		return epoch.Epoch{}, fmt.Errorf("publish Epoch: %w", err)
	}
	return published, nil
}

func epochEventID(source journal.EventID, eventType string) journal.EventID {
	digest := sha256.Sum256([]byte(string(source) + "\x00" + eventType))
	return journal.EventID("event/" + hex.EncodeToString(digest[:]))
}
