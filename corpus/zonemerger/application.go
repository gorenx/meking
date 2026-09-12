package zonemerger

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/zone"
)

var (
	ErrInvalidMerge = errors.New("invalid Child Corpus merge")
)

type ChildCorporaReader interface {
	ChildCorporaBoundaries(ctx context.Context) (map[string]corpus.CorporaID, error)
}

type CorporaReader interface {
	Corpora(ctx context.Context, id corpus.CorporaID) (corpus.Corpora, error)
}

type TextStore interface {
	Get(ctx context.Context, id text.ID) (text.Text, error)
	Save(ctx context.Context, value text.Text) (text.Text, error)
}

type Corpus interface {
	SaveTextUnits(ctx context.Context, value corpus.ChunkedText) (corpus.CorporaID, error)
	PrepareMergedTextUnits(ctx context.Context, preparation corpus.PrepareMergedTextUnitsInput) error
}

type Preparation struct {
	EventID       corpus.EventID
	CorrelationID corpus.CorrelationID
	OccurredAt    time.Time
}

type Dependencies struct {
	Boundaries ChildCorporaReader
	Corpora    CorporaReader
	Texts      TextStore
	Documents  *document.Manager
	Target     Corpus
}

type Application struct {
	dependencies Dependencies
}

func New(dependencies Dependencies) (*Application, error) {
	switch {
	case dependencies.Boundaries == nil:
		return nil, errors.New("create Corpus Zone Merger: Child Corpora boundaries are required")
	case dependencies.Corpora == nil:
		return nil, errors.New("create Corpus Zone Merger: Corpora reader is required")
	case dependencies.Texts == nil:
		return nil, errors.New("create Corpus Zone Merger: Text reader is required")
	case dependencies.Documents == nil:
		return nil, errors.New("create Corpus Zone Merger: Document Manager is required")
	case dependencies.Target == nil:
		return nil, errors.New("create Corpus Zone Merger: target Corpus is required")
	default:
		return &Application{dependencies: dependencies}, nil
	}
}

func (application *Application) MergeChildDocuments(ctx context.Context) error {
	return application.mergeCorpora(ctx,
		func(
			sourceContext context.Context,
			chunkedText corpus.ChunkedText,
		) error {
			value, err := application.dependencies.Texts.Get(sourceContext, chunkedText.TextID)
			if err != nil {
				return err
			}
			located, err := application.dependencies.Documents.Located(sourceContext, value.DocumentID)
			if err != nil {
				return err
			}
			recorded, err := application.dependencies.Documents.Record(ctx, located)
			if err != nil {
				return err
			}
			_, err = application.dependencies.Documents.Associate(
				ctx,
				[]document.ID{recorded.Document.ID},
			)
			return err
		},
	)
}

func (application *Application) MergeChildTexts(ctx context.Context) error {
	return application.mergeCorpora(ctx,
		func(
			sourceContext context.Context,
			chunkedText corpus.ChunkedText,
		) error {
			_, err := application.saveText(ctx, sourceContext, chunkedText.TextID)
			return err
		},
	)
}

func (application *Application) MergeTextUnits(ctx context.Context) error {
	_, _, err := application.mergeTextUnits(ctx)
	return err
}

func (application *Application) Prepare(ctx context.Context, preparation Preparation) error {
	if preparation.EventID == "" || preparation.CorrelationID == "" || preparation.OccurredAt.IsZero() {
		return ErrInvalidMerge
	}
	if err := application.MergeChildDocuments(ctx); err != nil {
		return err
	}
	if err := application.MergeChildTexts(ctx); err != nil {
		return err
	}
	corporaID, trigger, err := application.mergeTextUnits(ctx)
	if err != nil {
		return err
	}
	if trigger.TextID == "" {
		return fmt.Errorf("%w: accepted Child Corpora contain no Text", ErrInvalidMerge)
	}
	return application.dependencies.Target.PrepareMergedTextUnits(ctx, corpus.PrepareMergedTextUnitsInput{
		EventID: preparation.EventID, CorrelationID: preparation.CorrelationID,
		OccurredAt: preparation.OccurredAt, CorporaID: corporaID, TextID: trigger.TextID,
	})
}

func (application *Application) mergeTextUnits(ctx context.Context) (corpus.CorporaID, corpus.ChunkedText, error) {
	var corporaID corpus.CorporaID
	var trigger corpus.ChunkedText
	err := application.mergeCorpora(ctx, func(
		sourceContext context.Context,
		chunkedText corpus.ChunkedText,
	) error {
		saved, err := application.saveText(ctx, sourceContext, chunkedText.TextID)
		if err != nil {
			return err
		}
		id, err := application.saveTextUnits(ctx, saved.ID, chunkedText.TextUnits)
		if err != nil {
			return err
		}
		if trigger.TextID == "" {
			corporaID = id
			trigger = chunkedText
			trigger.TextID = saved.ID
		}
		return nil
	})
	return corporaID, trigger, err
}

func (application *Application) saveTextUnits(
	ctx context.Context,
	textID text.ID,
	units []textunits.TextUnit,
) (corpus.CorporaID, error) {
	set, err := corpus.NewChunkedText(textID, append([]textunits.TextUnit(nil), units...))
	if err != nil {
		return "", err
	}
	return application.dependencies.Target.SaveTextUnits(ctx, set)
}

func (application *Application) mergeCorpora(
	ctx context.Context,
	merge func(context.Context, corpus.ChunkedText) error,
) error {
	if application == nil || merge == nil {
		return ErrInvalidMerge
	}
	boundaries, err := application.dependencies.Boundaries.ChildCorporaBoundaries(ctx)
	if err != nil {
		return fmt.Errorf("read Child Corpus boundaries: %w", err)
	}
	childZoneIDs := make([]string, 0, len(boundaries))
	for childZoneID := range boundaries {
		childZoneIDs = append(childZoneIDs, childZoneID)
	}
	sort.Strings(childZoneIDs)
	for _, childZoneID := range childZoneIDs {
		sourceCorporaID := boundaries[childZoneID]
		childID, err := zone.ParseID(childZoneID)
		if err != nil {
			return fmt.Errorf("%w: invalid Child Zone %q", corpus.ErrCorpusDataIntegrity, childZoneID)
		}
		sourceContext, err := zone.RouteContext(ctx, childID)
		if err != nil {
			return fmt.Errorf("route Child Zone %q Corpus read: %w", childZoneID, err)
		}
		childValue, err := application.dependencies.Corpora.Corpora(sourceContext, sourceCorporaID)
		if err != nil {
			return fmt.Errorf("read Child Zone %q Corpora %q: %w", childZoneID, sourceCorporaID, err)
		}
		for _, chunkedText := range childValue.Texts {
			if err = merge(sourceContext, chunkedText); err != nil {
				return fmt.Errorf("merge Child Zone %q Corpus: %w", childZoneID, err)
			}
		}
	}
	return nil
}

func (application *Application) saveText(
	ctx context.Context,
	sourceContext context.Context,
	childTextID text.ID,
) (text.Text, error) {
	source, err := application.dependencies.Texts.Get(sourceContext, childTextID)
	if err != nil {
		return text.Text{}, err
	}
	located, err := application.dependencies.Documents.Located(sourceContext, source.DocumentID)
	if err != nil {
		return text.Text{}, err
	}
	documentResult, err := application.dependencies.Documents.Record(ctx, located)
	if err != nil {
		return text.Text{}, err
	}
	if _, err := application.dependencies.Documents.Associate(
		ctx,
		[]document.ID{documentResult.Document.ID},
	); err != nil {
		return text.Text{}, err
	}
	target := source
	target.DocumentID = documentResult.Document.ID
	return application.dependencies.Texts.Save(ctx, target)
}
