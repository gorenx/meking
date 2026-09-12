package journal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/extraction"
)

type CorporaExtractor interface {
	ExtractCorpora(context.Context, extraction.Request) (knowledge.Manifest, error)
}

type ExtractionActionDependencies struct {
	Journal    *journalcore.Service
	Positions  journalcore.ConsumerPositions
	Extraction CorporaExtractor
	ReadLimit  int
	Logger     *slog.Logger
}

type ExtractionAction struct {
	runtime    *actionruntime.Runtime
	extraction CorporaExtractor
	logger     *slog.Logger
}

func NewExtractionAction(
	dependencies ExtractionActionDependencies,
) (*ExtractionAction, error) {
	if dependencies.Extraction == nil {
		return nil, errors.New("create Knowledge Extraction Action: Extraction is required")
	}
	if dependencies.Logger == nil {
		return nil, errors.New("create Knowledge Extraction Action: Logger is required")
	}
	action := &ExtractionAction{
		extraction: dependencies.Extraction,
		logger:     dependencies.Logger,
	}
	runtime, err := newActionRuntime(
		"Knowledge Extraction",
		"knowledge.extraction",
		actionRuntimeDependencies{
			Journal:   dependencies.Journal,
			Positions: dependencies.Positions,
			ReadLimit: dependencies.ReadLimit,
		},
		journalcore.EventKeyFor[corpusevents.CorporaPreparedV1](),
		action.startExtraction,
	)
	if err != nil {
		return nil, err
	}
	action.runtime = runtime
	return action, nil
}

func (action *ExtractionAction) Pending(ctx context.Context) (actionruntime.PendingInput, error) {
	if action == nil {
		return actionruntime.PendingInput{}, errors.New("read pending Knowledge Extraction input: Action is required")
	}
	return action.runtime.Pending(ctx)
}

func (action *ExtractionAction) Start(ctx context.Context) error {
	if action == nil {
		return errors.New("start Knowledge Extraction: Action is required")
	}
	return action.runtime.Start(ctx)
}

func (action *ExtractionAction) Retry(ctx context.Context) error {
	if action == nil {
		return errors.New("retry Knowledge Extraction: Action is required")
	}
	return action.runtime.Retry(ctx)
}

func (action *ExtractionAction) Run(ctx context.Context) error {
	if action == nil {
		return errors.New("run Knowledge Extraction Action: Action is required")
	}
	return action.runtime.Run(ctx)
}

func (action *ExtractionAction) startExtraction(
	ctx context.Context,
	event journalcore.Event,
) error {
	body, err := journalcore.DecodeJSONBody[corpusevents.CorporaPreparedV1](event)
	if err != nil {
		return fmt.Errorf("decode Corpora for Knowledge Extraction: %w", err)
	}
	if err := corpusevents.ValidateCorporaStream(string(event.StreamID)); err != nil {
		return err
	}
	_, err = action.extraction.ExtractCorpora(ctx, extraction.Request{
		SourceEventID: event.EventID,
		CorrelationID: event.CorrelationID,
		RequestedAt:   event.OccurredAt,
		CorporaID:     string(body.CorporaID),
	})
	if err != nil {
		failure := fmt.Errorf("start Knowledge Extraction for Corpora %q: %w", body.CorporaID, err)
		if rejectedAgentResult(err) && ctx.Err() == nil {
			action.logger.ErrorContext(
				ctx,
				"Knowledge Extraction rejected Agent result",
				slog.String("zone_id", string(event.ZoneID)),
				slog.String("corpora_id", string(body.CorporaID)),
				slog.String("event_id", string(event.EventID)),
				slog.Any("error", failure),
			)
			return fmt.Errorf("%w: %w", actionruntime.ErrEventIncomplete, failure)
		}
		return failure
	}
	return nil
}

func rejectedAgentResult(err error) bool {
	return errors.Is(err, extraction.ErrAgentRequest) ||
		errors.Is(err, extraction.ErrInvalidGraphExtraction) ||
		errors.Is(err, extraction.ErrInvalidClaimExtraction)
}
