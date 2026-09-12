package corpus

import (
	"errors"
	"testing"

	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
)

func TestNewCorporaCanonicalizesChunkedTexts(t *testing.T) {
	unit, err := textunits.NewTextUnitBody("same")
	if err != nil {
		t.Fatalf("NewTextUnitBody() error = %v", err)
	}
	first, err := NewChunkedText(validTextID('a'), []textunits.TextUnit{{
		TextUnit: unit, StartIndex: 0, EndIndex: 4, TokenCount: 1,
	}})
	if err != nil {
		t.Fatalf("NewChunkedText(first) error = %v", err)
	}
	second, err := NewChunkedText(validTextID('b'), []textunits.TextUnit{{
		TextUnit: unit, StartIndex: 5, EndIndex: 9, TokenCount: 1,
	}})
	if err != nil {
		t.Fatalf("NewChunkedText(second) error = %v", err)
	}
	set, err := NewCorpora("1", []ChunkedText{second, first})
	if err != nil {
		t.Fatalf("NewCorpora() error = %v", err)
	}
	units := set.TextUnits()
	if set.TextCount() != 2 || set.UniqueTextUnitCount() != 1 || len(units) != 2 ||
		set.Texts[0].TextID != validTextID('a') || set.Texts[1].TextID != validTextID('b') {
		t.Fatalf("Corpora TextUnits = %#v", units)
	}
}

func TestNewChunkedTextRejectsDuplicateTextUnitRange(t *testing.T) {
	first, _ := textunits.NewTextUnitBody("first")
	second, _ := textunits.NewTextUnitBody("second")
	textID := validTextID('b')
	_, err := NewChunkedText(
		textID,
		[]textunits.TextUnit{
			{TextUnit: first, StartIndex: 0, EndIndex: 5, TokenCount: 1},
			{TextUnit: second, StartIndex: 0, EndIndex: 5, TokenCount: 1},
		},
	)
	if !errors.Is(err, ErrInvalidCorpus) {
		t.Fatalf("NewChunkedText() error = %v, want ErrInvalidCorpus", err)
	}
}

func validTextID(value byte) text.ID {
	if value == 'a' {
		return "11111111-1111-4111-8111-111111111111"
	}
	return "22222222-2222-4222-8222-222222222222"
}
