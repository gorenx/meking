package corpus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/memoria-space/meking/corpus/document"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
)

type (
	EventID       string
	StreamID      string
	CorrelationID string
)

// Event is a Corpus-owned fact ready for an outbound delivery adapter.
type Event struct {
	EventID       EventID
	StreamID      StreamID
	CorrelationID CorrelationID
	CausationID   EventID
	OccurredAt    time.Time
	Body          corpusevents.Body
}

// Producer publishes Corpus facts through the active transaction carried by ctx.
type Producer interface {
	Publish(ctx context.Context, events []Event) error
}

// Transaction executes one Corpus owner write and its outbound facts atomically.
type Transaction interface {
	WithTx(context.Context, func(context.Context) error) error
}
type CorporaID string

// ChunkedText is one Text's immutable TextUnit result inside a Corpora.
type ChunkedText struct {
	TextID    text.ID
	TextUnits []textunits.TextUnit
}

// Corpora is one immutable, publishable collection of ChunkedTexts.
type Corpora struct {
	ID    CorporaID
	Texts []ChunkedText
}

// TextUnitLocation is the read projection for one TextUnit occurrence and its source.
type TextUnitLocation struct {
	CorporaID        CorporaID
	TextID           text.ID
	TextTitle        string
	DocumentID       document.ID
	DocumentLocation document.Location
	TextUnit         textunits.TextUnit
}

func NewChunkedText(textID text.ID, units []textunits.TextUnit) (ChunkedText, error) {
	return restoreChunkedText(textID, units, ErrInvalidCorpus)
}

func NewCorpora(id CorporaID, texts []ChunkedText) (Corpora, error) {
	return restoreCorpora(id, texts, ErrInvalidCorpus)
}

func RestoreCorpora(set Corpora) (Corpora, error) {
	return restoreCorpora(set.ID, set.Texts, ErrCorpusDataIntegrity)
}

func (s Corpora) TextCount() int {
	return len(s.Texts)
}

func (s Corpora) TextUnits() []textunits.TextUnit {
	count := 0
	for _, value := range s.Texts {
		count += len(value.TextUnits)
	}
	result := make([]textunits.TextUnit, 0, count)
	for _, value := range s.Texts {
		result = append(result, value.TextUnits...)
	}
	return result
}

func (s Corpora) UniqueTextUnitCount() int {
	ids := make(map[textunits.TextUnitID]struct{})
	for _, unit := range s.TextUnits() {
		ids[unit.TextUnit.ID] = struct{}{}
	}
	return len(ids)
}

func (s Corpora) TextUnitSetDigest() string {
	return textUnitSetDigest(s.TextUnits())
}

func (s ChunkedText) UniqueTextUnitCount() int {
	ids := make(map[textunits.TextUnitID]struct{}, len(s.TextUnits))
	for _, unit := range s.TextUnits {
		ids[unit.TextUnit.ID] = struct{}{}
	}
	return len(ids)
}

func (s ChunkedText) TextUnitSetDigest() string {
	return textUnitSetDigest(s.TextUnits)
}

func textUnitSetDigest(units []textunits.TextUnit) string {
	hasher := sha256.New()
	for _, unit := range units {
		_, _ = hasher.Write([]byte(unit.TextUnit.ID))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func restoreCorpora(
	id CorporaID,
	texts []ChunkedText,
	failure error,
) (Corpora, error) {
	if err := ValidateCorporaID(id); err != nil {
		return Corpora{}, fmt.Errorf("%w: %v", failure, err)
	}
	if len(texts) == 0 {
		return Corpora{}, fmt.Errorf("%w: Corpora requires at least one ChunkedText", failure)
	}
	result := Corpora{ID: id, Texts: make([]ChunkedText, len(texts))}
	for index, value := range texts {
		restored, err := restoreChunkedText(value.TextID, value.TextUnits, failure)
		if err != nil {
			return Corpora{}, fmt.Errorf("%w: ChunkedText %d: %v", failure, index, err)
		}
		result.Texts[index] = restored
	}
	sort.Slice(result.Texts, func(left, right int) bool {
		return result.Texts[left].TextID < result.Texts[right].TextID
	})
	contentByID := make(map[textunits.TextUnitID]string)
	for index, value := range result.Texts {
		if index > 0 && result.Texts[index-1].TextID == value.TextID {
			return Corpora{}, fmt.Errorf("%w: duplicate ChunkedText %q", failure, value.TextID)
		}
		for _, unit := range value.TextUnits {
			if previous, found := contentByID[unit.TextUnit.ID]; found && previous != unit.TextUnit.Text {
				return Corpora{}, fmt.Errorf(
					"%w: TextUnitBody %q identifies different content",
					failure,
					unit.TextUnit.ID,
				)
			}
			contentByID[unit.TextUnit.ID] = unit.TextUnit.Text
		}
	}
	return result, nil
}

func restoreChunkedText(
	textID text.ID,
	units []textunits.TextUnit,
	failure error,
) (ChunkedText, error) {
	if err := text.ValidateID(textID); err != nil {
		return ChunkedText{}, fmt.Errorf("%w: %v", failure, err)
	}
	if len(units) == 0 {
		return ChunkedText{}, fmt.Errorf("%w: ChunkedText requires at least one TextUnit", failure)
	}
	result := ChunkedText{TextID: textID, TextUnits: append([]textunits.TextUnit(nil), units...)}
	sort.Slice(result.TextUnits, func(left, right int) bool {
		return lessTextUnit(result.TextUnits[left], result.TextUnits[right])
	})
	for index, unit := range result.TextUnits {
		if err := validateTextUnit(unit); err != nil {
			return ChunkedText{}, fmt.Errorf("%w: TextUnit %d: %v", failure, index, err)
		}
		if index > 0 && sameTextUnitRange(result.TextUnits[index-1], unit) {
			return ChunkedText{}, fmt.Errorf(
				"%w: Text %q has duplicate TextUnit range [%d,%d)",
				failure,
				textID,
				unit.StartIndex,
				unit.EndIndex,
			)
		}
	}
	return result, nil
}

func lessTextUnit(left, right textunits.TextUnit) bool {
	if left.StartIndex != right.StartIndex {
		return left.StartIndex < right.StartIndex
	}
	return left.EndIndex < right.EndIndex
}

func sameTextUnitRange(left, right textunits.TextUnit) bool {
	return left.StartIndex == right.StartIndex &&
		left.EndIndex == right.EndIndex
}
