package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/textunits/vectorindex"
	journalcore "github.com/memoria-space/meking/journal"
)

type TextUnitVectorBuilder interface {
	Build(ctx context.Context, prepared vectorindex.PreparedTextUnits) error
}

type TextUnitVectorConsumer struct {
	builder TextUnitVectorBuilder
}

var _ journalcore.Consumer = (*TextUnitVectorConsumer)(nil)

func NewTextUnitVectorConsumer(builder TextUnitVectorBuilder) (*TextUnitVectorConsumer, error) {
	if builder == nil {
		return nil, errors.New("create Corpus TextUnit vector consumer: Builder is required")
	}
	return &TextUnitVectorConsumer{builder: builder}, nil
}

func (consumer *TextUnitVectorConsumer) ConsumerRegistration() journalcore.ConsumerRegistration {
	return journalcore.ConsumerRegistration{
		ID: "corpus.text-unit-vector",
		Events: []journalcore.EventKey{journalcore.EventKeyFor[corpusevents.TextUnitsPreparedV1]()},
	}
}

func (consumer *TextUnitVectorConsumer) Handle(ctx context.Context, event journalcore.Event) error {
	body, err := journalcore.DecodeJSONBody[corpusevents.TextUnitsPreparedV1](event)
	if err != nil {
		return fmt.Errorf("decode prepared TextUnits for vectors: %w", err)
	}
	if err := consumer.builder.Build(ctx, vectorindex.PreparedTextUnits{
		EventID: corpus.EventID(event.EventID), StreamID: corpus.StreamID(event.StreamID),
		CorrelationID: corpus.CorrelationID(event.CorrelationID), OccurredAt: event.OccurredAt, Body: body,
	}); err != nil {
		return fmt.Errorf("build TextUnit vectors for Text %q: %w", body.TextID, err)
	}
	return nil
}
