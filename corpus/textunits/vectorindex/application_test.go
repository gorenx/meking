package vectorindex

import (
	"context"
	"testing"
	"time"

	"github.com/memoria-space/meking/corpus"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
)

func TestApplicationUsesUniqueTextUnitCountAndRetainsSpanCount(t *testing.T) {
	unit, err := textunits.NewTextUnitBody("same")
	if err != nil {
		t.Fatal(err)
	}
	first := text.ID("11111111-1111-4111-8111-111111111111")
	second := text.ID("22222222-2222-4222-8222-222222222222")
	firstText, err := corpus.NewChunkedText(first, []textunits.TextUnit{{
		TextUnit: unit, StartIndex: 0, EndIndex: 4, TokenCount: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	secondText, err := corpus.NewChunkedText(second, []textunits.TextUnit{{
		TextUnit: unit, StartIndex: 0, EndIndex: 4, TokenCount: 1,
	}, {
		TextUnit: unit, StartIndex: 5, EndIndex: 9, TokenCount: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	set, err := corpus.NewCorpora("1", []corpus.ChunkedText{firstText, secondText})
	if err != nil {
		t.Fatal(err)
	}
	completer := &recordingCompleter{}
	application, err := NewApplication(ApplicationDependencies{
		Corpora: fixedBuildingCorpora{set: set}, Builder: fixedBuilder{count: 1},
		Completer: completer,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := corpusevents.TextUnitsPreparedV1{
		CorporaID:         corpusevents.CorporaID(corpus.FormatCorporaID(set.ID)),
		TextID:            corpusevents.TextID(second),
		TextUnitSetDigest: secondText.TextUnitSetDigest(), TextUnitCount: 1, TextUnitSpanCount: 2,
	}
	err = application.Build(t.Context(), PreparedTextUnits{
		EventID: "prepared", StreamID: corpus.StreamID(corpusevents.TextStream(body.TextID)),
		CorrelationID: "request", OccurredAt: time.Now().UTC(), Body: body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !completer.completed || completer.completion.CorporaID != set.ID ||
		completer.completion.TextID != second || completer.completion.EventID != "prepared" {
		t.Fatalf("completion = %#v, completed = %v", completer.completion, completer.completed)
	}
}

type fixedBuildingCorpora struct{ set corpus.Corpora }

func (source fixedBuildingCorpora) BuildingCorpora(
	context.Context,
	corpus.CorporaID,
) (corpus.Corpora, error) {
	return source.set, nil
}

type fixedBuilder struct{ count int }

func (builder fixedBuilder) Build(context.Context, string, []textunits.TextUnitBody) (int, error) {
	return builder.count, nil
}

type recordingCompleter struct {
	completion corpus.TextVectorCompletion
	completed  bool
}

func (completer *recordingCompleter) RecordTextUnitVectors(
	_ context.Context,
	value corpus.TextVectorCompletion,
) (bool, error) {
	completer.completion = value
	return true, nil
}

func (completer *recordingCompleter) CompleteCorpora(
	_ context.Context,
	value corpus.TextVectorCompletion,
) error {
	completer.completion = value
	completer.completed = true
	return nil
}
