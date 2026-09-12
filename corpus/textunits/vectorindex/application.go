package vectorindex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/memoria-space/meking/corpus"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
)

var (
	ErrInvalidPreparedTextUnits = errors.New("invalid prepared TextUnits")
	ErrPreparedTextConflict     = errors.New("prepared TextUnits conflict with persisted Corpora")
)

type PreparedTextUnits struct {
	EventID       corpus.EventID
	StreamID      corpus.StreamID
	CorrelationID corpus.CorrelationID
	OccurredAt    time.Time
	Body          corpusevents.TextUnitsPreparedV1
}

type BuildingCorporaReader interface {
	BuildingCorpora(ctx context.Context, corporaID corpus.CorporaID) (corpus.Corpora, error)
}

type VectorBuilder interface {
	Build(ctx context.Context, corporaID string, units []textunits.TextUnitBody) (int, error)
}

type Completer interface {
	RecordTextUnitVectors(ctx context.Context, completion corpus.TextVectorCompletion) (bool, error)
	CompleteCorpora(ctx context.Context, completion corpus.TextVectorCompletion) error
}

type ApplicationDependencies struct {
	Corpora   BuildingCorporaReader
	Builder   VectorBuilder
	Completer Completer
}

type Application struct {
	dependencies ApplicationDependencies
}

func NewApplication(dependencies ApplicationDependencies) (*Application, error) {
	switch {
	case dependencies.Corpora == nil:
		return nil, errors.New("create Corpus TextUnit vector Application: Building Corpora are required")
	case dependencies.Builder == nil:
		return nil, errors.New("create Corpus TextUnit vector Application: Builder is required")
	case dependencies.Completer == nil:
		return nil, errors.New("create Corpus TextUnit vector Application: Completer is required")
	default:
		return &Application{dependencies: dependencies}, nil
	}
}

func (application *Application) Build(ctx context.Context, prepared PreparedTextUnits) error {
	if err := validatePreparedTextUnits(prepared); err != nil {
		return err
	}
	corporaID, err := corpus.ParseCorporaID(string(prepared.Body.CorporaID))
	if err != nil {
		return ErrInvalidPreparedTextUnits
	}
	textID := text.ID(prepared.Body.TextID)
	set, err := application.dependencies.Corpora.BuildingCorpora(ctx, corporaID)
	if err != nil {
		return fmt.Errorf("load Building Corpora for TextUnit vectors: %w", err)
	}
	if err = verifyPreparedText(set, prepared.Body); err != nil {
		return err
	}
	spans := set.TextUnits()
	units := make([]textunits.TextUnitBody, len(spans))
	for index, span := range spans {
		units[index] = span.TextUnit
	}
	count, err := application.dependencies.Builder.Build(
		ctx,
		corpus.FormatCorporaID(corporaID),
		units,
	)
	if err != nil {
		return fmt.Errorf("build Building Corpora TextUnit vectors: %w", err)
	}
	if count != set.UniqueTextUnitCount() {
		return ErrPreparedTextConflict
	}
	completion := corpus.TextVectorCompletion{
		EventID: prepared.EventID, CorrelationID: prepared.CorrelationID,
		OccurredAt: prepared.OccurredAt, CorporaID: corporaID, TextID: textID,
	}
	ready, err := application.dependencies.Completer.RecordTextUnitVectors(ctx, completion)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	if err := application.dependencies.Completer.CompleteCorpora(ctx, completion); err != nil {
		return fmt.Errorf("complete Corpora after TextUnit vectors: %w", err)
	}
	return nil
}

func validatePreparedTextUnits(value PreparedTextUnits) error {
	if strings.TrimSpace(string(value.EventID)) == "" || strings.TrimSpace(string(value.StreamID)) == "" ||
		strings.TrimSpace(string(value.CorrelationID)) == "" || value.OccurredAt.IsZero() {
		return ErrInvalidPreparedTextUnits
	}
	if err := corpusevents.ValidateTextStream(string(value.StreamID)); err != nil || value.Body.Validate() != nil {
		return ErrInvalidPreparedTextUnits
	}
	return nil
}

func verifyPreparedText(set corpus.Corpora, body corpusevents.TextUnitsPreparedV1) error {
	for _, value := range set.Texts {
		if value.TextID != text.ID(body.TextID) {
			continue
		}
		if value.TextUnitSetDigest() != body.TextUnitSetDigest ||
			uint64(value.UniqueTextUnitCount()) != body.TextUnitCount ||
			uint64(len(value.TextUnits)) != body.TextUnitSpanCount {
			return ErrPreparedTextConflict
		}
		return nil
	}
	return ErrPreparedTextConflict
}
