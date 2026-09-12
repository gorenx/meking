package corpus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/corpus/document"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/zone"
)

type TextPreparation struct {
	EventID       EventID
	CorrelationID CorrelationID
	OccurredAt    time.Time
	DocumentID    document.ID
}

func (s *Service) PrepareText(ctx context.Context, preparation TextPreparation) (text.Text, error) {
	if s == nil || s.transactions == nil || s.producer == nil || s.texts == nil {
		return text.Text{}, errors.New("Corpus Text preparations are not configured")
	}
	if preparation.EventID == "" || preparation.CorrelationID == "" || preparation.OccurredAt.IsZero() {
		return text.Text{}, errors.New("prepare Corpus Text: EventID, CorrelationID, and OccurredAt are required")
	}
	if err := document.ValidateID(preparation.DocumentID); err != nil {
		return text.Text{}, fmt.Errorf("prepare Corpus Text: %w", err)
	}
	prepared, err := s.texts.Prepare(ctx, preparation.DocumentID)
	if err != nil {
		return text.Text{}, fmt.Errorf("prepare Corpus Document %q Text: %w", preparation.DocumentID, err)
	}
	result := prepared
	err = s.transactions.WithTx(ctx, func(ctx context.Context) error {
		saved, err := s.texts.Save(ctx, prepared)
		if err != nil {
			return fmt.Errorf("save prepared Corpora Text: %w", err)
		}
		result = saved
		zoneID, err := zone.RequireID(ctx)
		if err != nil {
			return err
		}
		body := corpusevents.TextPreparedV1{
			DocumentID: corpusevents.DocumentID(saved.DocumentID), TextID: corpusevents.TextID(saved.ID),
			Source: corpusevents.TextSource{ZoneID: string(zoneID), TextID: corpusevents.TextID(saved.ID)},
		}
		return s.producer.Publish(ctx, []Event{{
			EventID:       preparedTextEventID(preparation.EventID),
			StreamID:      StreamID(corpusevents.TextStream(body.TextID)),
			CorrelationID: preparation.CorrelationID, CausationID: preparation.EventID,
			OccurredAt: preparation.OccurredAt.UTC(), Body: body,
		}})
	})
	if err != nil {
		return text.Text{}, fmt.Errorf("commit Corpus Text preparation: %w", err)
	}
	return result, nil
}

func preparedTextEventID(source EventID) EventID {
	digest := sha256.Sum256([]byte(source))
	return EventID("text-prepared/" + hex.EncodeToString(digest[:]))
}
