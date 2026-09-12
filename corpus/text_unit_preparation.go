package corpus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
)

const textUnitWriteBatchSize = 128

// TextChunkingProgress is the recovery position for one Text in a Building Corpus.
// NextChunkIndex is the number of ordered chunks already committed.
type TextChunkingProgress struct {
	TextID         text.ID
	NextChunkIndex int
	Completed      bool
}

type PrepareTextUnitsInput struct {
	EventID       EventID
	CorrelationID CorrelationID
	OccurredAt    time.Time
	TextID        text.ID
}

type PrepareMergedTextUnitsInput struct {
	EventID       EventID
	CorrelationID CorrelationID
	OccurredAt    time.Time
	CorporaID     CorporaID
	TextID        text.ID
}

func (s *Service) PrepareTextUnits(
	ctx context.Context,
	preparation PrepareTextUnitsInput,
) (CorporaID, error) {
	if s == nil || s.transactions == nil || s.producer == nil {
		return "", errors.New("Corpus TextUnit preparations are not configured")
	}
	if preparation.EventID == "" || preparation.CorrelationID == "" || preparation.OccurredAt.IsZero() {
		return "", errors.New("prepare Corpus TextUnits: EventID, CorrelationID, and OccurredAt are required")
	}
	if err := text.ValidateID(preparation.TextID); err != nil {
		return "", fmt.Errorf("prepare Corpus TextUnits: %w", err)
	}
	var corporaID CorporaID
	var progress TextChunkingProgress
	err := s.transactions.WithTx(ctx, func(ctx context.Context) error {
		var err error
		corporaID, err = s.store.BeginCorporaBuild(ctx)
		if err != nil {
			return err
		}
		progress, err = s.store.OpenTextChunkingProgress(
			ctx, corporaID, preparation.EventID, preparation.TextID,
		)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("open Corpus Text chunking progress: %w", err)
	}
	value, err := s.store.TextStore().Get(ctx, preparation.TextID)
	if err != nil {
		return "", fmt.Errorf("read Corpus Text %q: %w", preparation.TextID, err)
	}
	builder, err := textunits.NewTextUnitBuilder(s.chunking, s.sentenceAnalyzer)
	if err != nil {
		return "", fmt.Errorf("create TextUnit builder: %w", err)
	}
	outputs, err := builder.Build(ctx, value)
	if err != nil {
		return "", err
	}
	if len(outputs) == 0 {
		return "", fmt.Errorf("%w: Text %q produced no TextUnits", ErrInvalidCorpus, preparation.TextID)
	}
	set, err := NewChunkedText(preparation.TextID, outputs)
	if err != nil {
		return "", fmt.Errorf("validate prepared TextUnits: %w", err)
	}
	return s.recordTextUnits(
		ctx, corporaID, progress, preparation.EventID, preparation.EventID,
		preparation.CorrelationID, preparation.OccurredAt, set,
	)
}

func (s *Service) SaveTextUnits(ctx context.Context, value ChunkedText) (CorporaID, error) {
	if s == nil || s.transactions == nil {
		return "", errors.New("Corpus TextUnit persistence is not configured")
	}
	prepared, err := NewChunkedText(value.TextID, value.TextUnits)
	if err != nil {
		return "", err
	}
	var corporaID CorporaID
	err = s.transactions.WithTx(ctx, func(ctx context.Context) error {
		var err error
		corporaID, err = s.store.BeginCorporaBuild(ctx)
		if err != nil {
			return err
		}
		stored, found, err := s.store.LoadChunkedText(ctx, corporaID, prepared.TextID)
		if err != nil {
			return err
		}
		if found {
			if !slices.Equal(stored.TextUnits, prepared.TextUnits) {
				return ErrCorporaPreparationConflict
			}
			return nil
		}
		return s.store.SaveTextUnits(ctx, corporaID, prepared)
	})
	if err != nil {
		return "", fmt.Errorf("save Corpus TextUnits: %w", err)
	}
	return corporaID, nil
}

func (s *Service) PrepareMergedTextUnits(ctx context.Context, preparation PrepareMergedTextUnitsInput) error {
	if s == nil || s.transactions == nil || s.producer == nil {
		return errors.New("Corpus merged TextUnit preparations are not configured")
	}
	if preparation.EventID == "" || preparation.CorrelationID == "" || preparation.OccurredAt.IsZero() ||
		ValidateCorporaID(preparation.CorporaID) != nil || text.ValidateID(preparation.TextID) != nil {
		return ErrCorporaPreparationConflict
	}
	return s.transactions.WithTx(ctx, func(ctx context.Context) error {
		set, found, err := s.store.LoadChunkedText(ctx, preparation.CorporaID, preparation.TextID)
		if err != nil {
			return err
		}
		if !found {
			return ErrCorporaPreparationConflict
		}
		return s.producer.Publish(ctx, []Event{textUnitsPreparedEvent(
			preparation.EventID,
			preparation.EventID,
			preparation.CorrelationID,
			preparation.OccurredAt,
			preparation.CorporaID,
			set,
		)})
	})
}

func (s *Service) recordTextUnits(
	ctx context.Context,
	corporaID CorporaID,
	progress TextChunkingProgress,
	sourceEventID EventID,
	causationID EventID,
	correlationID CorrelationID,
	occurredAt time.Time,
	set ChunkedText,
) (CorporaID, error) {
	if progress.NextChunkIndex > len(set.TextUnits) || progress.Completed && progress.NextChunkIndex != len(set.TextUnits) {
		return "", ErrCorporaPreparationConflict
	}
	if progress.Completed {
		return corporaID, nil
	}
	if progress.NextChunkIndex == len(set.TextUnits) {
		return "", ErrCorporaPreparationConflict
	}
	next := progress.NextChunkIndex
	for len(set.TextUnits)-next > textUnitWriteBatchSize {
		end := next + textUnitWriteBatchSize
		batchProgress := TextChunkingProgress{TextID: progress.TextID, NextChunkIndex: next}
		if err := s.transactions.WithTx(ctx, func(ctx context.Context) error {
			if err := s.store.SaveTextUnitBatch(
				ctx, corporaID, batchProgress.TextID, set.TextUnits[next:end],
			); err != nil {
				return err
			}
			return s.store.AdvanceTextChunkingProgress(ctx, corporaID, batchProgress, end, false)
		}); err != nil {
			return "", fmt.Errorf("commit Corpus TextUnit batch: %w", err)
		}
		next = end
	}
	finalProgress := TextChunkingProgress{TextID: progress.TextID, NextChunkIndex: next}
	err := s.transactions.WithTx(ctx, func(ctx context.Context) error {
		if err := s.store.SaveTextUnitBatch(
			ctx, corporaID, finalProgress.TextID, set.TextUnits[next:],
		); err != nil {
			return fmt.Errorf("save final TextUnit batch: %w", err)
		}
		if err := s.store.AdvanceTextChunkingProgress(
			ctx, corporaID, finalProgress, len(set.TextUnits), true,
		); err != nil {
			return fmt.Errorf("complete Text chunking progress: %w", err)
		}
		return s.producer.Publish(ctx, []Event{textUnitsPreparedEvent(
			sourceEventID, causationID, correlationID, occurredAt, corporaID, set,
		)})
	})
	if err != nil {
		return "", fmt.Errorf("commit final Corpus TextUnit batch: %w", err)
	}
	return corporaID, nil
}

func textUnitsPreparedEvent(
	sourceEventID EventID,
	causationID EventID,
	correlationID CorrelationID,
	occurredAt time.Time,
	corporaID CorporaID,
	set ChunkedText,
) Event {
	body := corpusevents.TextUnitsPreparedV1{
		CorporaID: corpusevents.CorporaID(FormatCorporaID(corporaID)), TextID: corpusevents.TextID(set.TextID),
		TextUnitSetDigest: set.TextUnitSetDigest(), TextUnitCount: uint64(set.UniqueTextUnitCount()),
		TextUnitSpanCount: uint64(len(set.TextUnits)),
	}
	return Event{
		EventID: preparedTextUnitsEventID(sourceEventID), StreamID: StreamID(corpusevents.TextStream(body.TextID)),
		CorrelationID: correlationID, CausationID: causationID, OccurredAt: occurredAt.UTC(), Body: body,
	}
}

func preparedTextUnitsEventID(source EventID) EventID {
	digest := sha256.Sum256([]byte(source))
	return EventID("text-units-prepared/" + hex.EncodeToString(digest[:]))
}
