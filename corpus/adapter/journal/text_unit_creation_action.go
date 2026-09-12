package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
)

type TextUnitPreparer interface {
	PrepareTextUnits(context.Context, corpus.PrepareTextUnitsInput) (corpus.CorporaID, error)
}

type TextUnitCreationActionDependencies struct {
	Journal      *journalcore.Service
	Positions    journalcore.ConsumerPositions
	TextUnits    TextUnitPreparer
	Merger       ZoneMerger
	ReadLimit    int
}

type TextUnitCreationAction struct {
	runtime   *actionruntime.Runtime
	textUnits TextUnitPreparer
	merger    ZoneMerger
}

func NewTextUnitCreationAction(
	dependencies TextUnitCreationActionDependencies,
) (*TextUnitCreationAction, error) {
	switch {
	case dependencies.TextUnits == nil:
		return nil, errors.New("create TextUnit creation Action: TextUnit Preparer is required")
	case dependencies.Merger == nil:
		return nil, errors.New("create TextUnit creation Action: Zone Merger is required")
	}
	action := &TextUnitCreationAction{
		textUnits: dependencies.TextUnits,
		merger:    dependencies.Merger,
	}
	runtime, err := newActionRuntime(
		"TextUnit creation",
		"corpus.text-unit-creation",
		actionRuntimeDependencies{
			Journal:   dependencies.Journal,
			Positions: dependencies.Positions,
			ReadLimit: dependencies.ReadLimit,
		},
		journalcore.EventKeyFor[corpusevents.TextPreparedV1](),
		action.createTextUnits,
	)
	if err != nil {
		return nil, err
	}
	action.runtime = runtime
	return action, nil
}

func (action *TextUnitCreationAction) Pending(ctx context.Context) (actionruntime.PendingInput, error) {
	if action == nil {
		return actionruntime.PendingInput{}, errors.New("read pending TextUnit creation input: Action is required")
	}
	return action.runtime.Pending(ctx)
}

func (action *TextUnitCreationAction) Start(ctx context.Context) error {
	if action == nil {
		return errors.New("start TextUnit creation: Action is required")
	}
	return action.runtime.Start(ctx)
}

func (action *TextUnitCreationAction) Retry(ctx context.Context) error {
	if action == nil {
		return errors.New("retry TextUnit creation: Action is required")
	}
	return action.runtime.Retry(ctx)
}

func (action *TextUnitCreationAction) Run(ctx context.Context) error {
	if action == nil {
		return errors.New("run TextUnit creation Action: Action is required")
	}
	return action.runtime.Run(ctx)
}

func (action *TextUnitCreationAction) createTextUnits(
	ctx context.Context,
	event journalcore.Event,
) error {
	body, err := journalcore.DecodeJSONBody[corpusevents.TextPreparedV1](event)
	if err != nil {
		return fmt.Errorf("decode Corpus Text event: %w", err)
	}
	if event.StreamID != journalcore.StreamID(corpusevents.TextStream(body.TextID)) ||
		body.Source.ZoneID != string(event.ZoneID) {
		return fmt.Errorf("Corpus Text event %q has inconsistent source", event.EventID)
	}
	if err := action.merger.MergeTextUnits(ctx); err != nil {
		return fmt.Errorf("merge Child Corpus TextUnits: %w", err)
	}
	_, err = action.textUnits.PrepareTextUnits(ctx, corpus.PrepareTextUnitsInput{
		EventID:       corpus.EventID(event.EventID),
		CorrelationID: corpus.CorrelationID(event.CorrelationID),
		OccurredAt:    event.OccurredAt,
		TextID:        text.ID(body.TextID),
	})
	if err != nil {
		return fmt.Errorf("prepare Corpus Text %q TextUnits: %w", body.TextID, err)
	}
	return nil
}
