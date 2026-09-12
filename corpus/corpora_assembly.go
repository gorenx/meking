package corpus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/text"
)

type TextVectorCompletion struct {
	EventID       EventID
	CorrelationID CorrelationID
	OccurredAt    time.Time
	CorporaID     CorporaID
	TextID        text.ID
}

func (service *Service) RecordTextUnitVectors(
	ctx context.Context,
	completion TextVectorCompletion,
) (bool, error) {
	if service == nil || service.transactions == nil {
		return false, errors.New("Corpus vector completion is not configured")
	}
	if completion.EventID == "" || completion.CorrelationID == "" || completion.OccurredAt.IsZero() ||
		ValidateCorporaID(completion.CorporaID) != nil || text.ValidateID(completion.TextID) != nil {
		return false, ErrCorporaPreparationConflict
	}
	ready := false
	if err := service.transactions.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ready, err = service.store.RecordTextUnitVectors(ctx, completion)
		return err
	}); err != nil {
		return false, fmt.Errorf("record Corpus TextUnit vectors: %w", err)
	}
	return ready, nil
}

func (service *Service) CompleteCorpora(
	ctx context.Context,
	completion TextVectorCompletion,
) error {
	if service == nil || service.transactions == nil || service.producer == nil {
		return errors.New("Corpus completion is not configured")
	}
	if completion.EventID == "" || completion.CorrelationID == "" || completion.OccurredAt.IsZero() ||
		ValidateCorporaID(completion.CorporaID) != nil {
		return ErrCorporaPreparationConflict
	}
	return service.transactions.WithTx(ctx, func(ctx context.Context) error {
		value, prepared, err := service.store.FinalizeCorpora(ctx, completion.CorporaID)
		if err != nil {
			return err
		}
		if !prepared {
			current, currentErr := service.store.CurrentID(ctx)
			if currentErr == nil && current == completion.CorporaID {
				return nil
			}
			return ErrCorporaPreparationConflict
		}
		if err = service.store.Activate(ctx, completion.CorporaID); err != nil {
			return err
		}
		body := corpusevents.CorporaPreparedV1{
			CorporaID: corpusevents.CorporaID(FormatCorporaID(value.ID)),
			TextCount: uint64(value.TextCount()), TextUnitSetDigest: value.TextUnitSetDigest(),
			TextUnitCount: uint64(value.UniqueTextUnitCount()), TextUnitSpanCount: uint64(len(value.TextUnits())),
		}
		return service.producer.Publish(ctx,
			[]Event{
				{
					EventID:       preparedCorporaEventID(completion.EventID, value.ID),
					StreamID:      StreamID(corpusevents.CorporaStream(body.CorporaID)),
					CorrelationID: completion.CorrelationID, CausationID: completion.EventID,
					OccurredAt: completion.OccurredAt.UTC(), Body: body,
				},
			},
		)
	})
}

func preparedCorporaEventID(sourceEventID EventID, id CorporaID) EventID {
	digest := sha256.Sum256([]byte(string(sourceEventID) + "\x00" + FormatCorporaID(id)))
	return EventID("corpora-prepared/" + hex.EncodeToString(digest[:]))
}
