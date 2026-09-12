package adapter

import (
	"context"
	"reflect"
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/textunits"
)

type corpusStub struct {
	setID       corpus.CorporaID
	textUnitIDs []textunits.TextUnitID
	locations   []corpus.TextUnitLocation
}

func (s *corpusStub) TextUnitLocations(
	_ context.Context,
	setID corpus.CorporaID,
	ids []textunits.TextUnitID,
) ([]corpus.TextUnitLocation, error) {
	s.setID = setID
	s.textUnitIDs = append([]textunits.TextUnitID(nil), ids...)
	return append([]corpus.TextUnitLocation(nil), s.locations...), nil
}

func TestSourceReaderMapsExactCorpusLocations(t *testing.T) {
	provider := &corpusStub{locations: []corpus.TextUnitLocation{{
		CorporaID: "corpora",
		TextID:    "text-id", DocumentID: "document-id", DocumentLocation: "input/source.txt", TextTitle: "Title",
		TextUnit: textunits.TextUnit{
			TextUnit: textunits.TextUnitBody{
				ID: "unit-id", Text: "evidence",
			},
			StartIndex: 0, EndIndex: 8, TokenCount: 1,
		},
	}}}
	reader, err := NewSourceReader(provider)
	if err != nil {
		t.Fatalf("NewSourceReader() error = %v", err)
	}
	result, err := reader.Read(t.Context(), "corpora", []string{"unit-id"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if provider.setID != "corpora" || !reflect.DeepEqual(provider.textUnitIDs, []textunits.TextUnitID{"unit-id"}) {
		t.Fatalf("provider request = %q %#v", provider.setID, provider.textUnitIDs)
	}
	if len(result) != 1 || result[0].TextUnitID != "unit-id" || result[0].Text != "evidence" ||
		result[0].DocumentLocation != "input/source.txt" {
		t.Fatalf("Read() = %#v", result)
	}
}
