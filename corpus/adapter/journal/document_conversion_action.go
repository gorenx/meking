package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
)

type DocumentTextPreparer interface {
	PrepareText(ctx context.Context, preparation corpus.TextPreparation) (text.Text, error)
}

type DocumentConversionActionDependencies struct {
	Journal      *journalcore.Service
	Positions    journalcore.ConsumerPositions
	Texts        DocumentTextPreparer
	Merger       ZoneMerger
	ReadLimit    int
}

type DocumentConversionAction struct {
	runtime *actionruntime.Runtime
	texts   DocumentTextPreparer
	merger  ZoneMerger
}

func NewDocumentConversionAction(
	dependencies DocumentConversionActionDependencies,
) (*DocumentConversionAction, error) {
	switch {
	case dependencies.Texts == nil:
		return nil, errors.New("create Document conversion Action: Text Preparer is required")
	case dependencies.Merger == nil:
		return nil, errors.New("create Document conversion Action: Zone Merger is required")
	}
	action := &DocumentConversionAction{
		texts:  dependencies.Texts,
		merger: dependencies.Merger,
	}
	runtime, err := newActionRuntime(
		"Document conversion",
		"corpus.document-conversion",
		actionRuntimeDependencies{
			Journal:   dependencies.Journal,
			Positions: dependencies.Positions,
			ReadLimit: dependencies.ReadLimit,
		},
		journalcore.EventKeyFor[corpusevents.DocumentRecordedV1](),
		action.convertDocument,
	)
	if err != nil {
		return nil, err
	}
	action.runtime = runtime
	return action, nil
}

func (action *DocumentConversionAction) Pending(ctx context.Context) (actionruntime.PendingInput, error) {
	if action == nil {
		return actionruntime.PendingInput{}, errors.New("read pending Document conversion input: Action is required")
	}
	return action.runtime.Pending(ctx)
}

func (action *DocumentConversionAction) Start(ctx context.Context) error {
	if action == nil {
		return errors.New("start Document conversion: Action is required")
	}
	return action.runtime.Start(ctx)
}

func (action *DocumentConversionAction) Retry(ctx context.Context) error {
	if action == nil {
		return errors.New("retry Document conversion: Action is required")
	}
	return action.runtime.Retry(ctx)
}

func (action *DocumentConversionAction) Run(ctx context.Context) error {
	if action == nil {
		return errors.New("run Document conversion Action: Action is required")
	}
	return action.runtime.Run(ctx)
}

func (action *DocumentConversionAction) convertDocument(
	ctx context.Context,
	event journalcore.Event,
) error {
	body, err := journalcore.DecodeJSONBody[corpusevents.DocumentRecordedV1](event)
	if err != nil {
		return fmt.Errorf("decode Corpus Document event: %w", err)
	}
	if event.StreamID != journalcore.StreamID(corpusevents.DocumentStream(body.DocumentID)) ||
		body.Source.ZoneID != string(event.ZoneID) {
		return fmt.Errorf("Corpus Document event %q has inconsistent source", event.EventID)
	}
	if err := action.merger.MergeChildTexts(ctx); err != nil {
		return fmt.Errorf("merge Child Corpus Texts: %w", err)
	}
	_, err = action.texts.PrepareText(ctx, corpus.TextPreparation{
		EventID:       corpus.EventID(event.EventID),
		CorrelationID: corpus.CorrelationID(event.CorrelationID),
		OccurredAt:    event.OccurredAt,
		DocumentID:    document.ID(body.DocumentID),
	})
	if err != nil {
		return fmt.Errorf("prepare Corpus Document %q Text: %w", body.DocumentID, err)
	}
	return nil
}
