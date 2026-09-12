// Package corpus adapts Corpora reads to Knowledge Extraction TextUnits.
package corpus

import (
	"context"
	"errors"
	"fmt"

	corpusdomain "github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/knowledge/extraction"
)

type Reader struct {
	corpus *corpusdomain.Service
}

var _ extraction.TextUnitReader = (*Reader)(nil)

func NewReader(corpus *corpusdomain.Service) (*Reader, error) {
	if corpus == nil {
		return nil, errors.New("create Knowledge Extraction Corpus Reader: Corpus Service is required")
	}
	return &Reader{corpus: corpus}, nil
}

func (reader *Reader) TextUnits(
	ctx context.Context,
	corporaID string,
) ([]extraction.TextUnitInput, error) {
	set, err := reader.corpus.Corpora(ctx, corpusdomain.CorporaID(corporaID))
	if err != nil {
		return nil, fmt.Errorf("read fixed Corpora: %w", err)
	}
	if string(set.ID) != corporaID {
		return nil, extraction.ErrExtractionProgress
	}
	seen := make(map[string]struct{})
	var units []extraction.TextUnitInput
	for _, span := range set.TextUnits() {
		id := string(span.TextUnit.ID)
		if _, found := seen[id]; found {
			continue
		}
		seen[id] = struct{}{}
		units = append(units, extraction.TextUnitInput{ID: id, Text: span.TextUnit.Text})
	}
	return units, nil
}
