// Package corpus maps immutable Corpora membership into the Community
// Report Builder contract without exposing Corpus storage types to Community.
package corpus

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/memoria-space/meking/community/reportgeneration"
	corpusbase "github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/textunits"
)

type Reader struct {
	source *corpusbase.Service
}

func NewReader(source *corpusbase.Service) (*Reader, error) {
	if source == nil {
		return nil, errors.New("create Community Corpus Reader: source is required")
	}
	return &Reader{source: source}, nil
}

func (r *Reader) Corpora(
	ctx context.Context,
	id string,
) (reportgeneration.CorpusSnapshot, error) {
	if r == nil || r.source == nil {
		return reportgeneration.CorpusSnapshot{}, errors.New(
			"read Community Corpora: Reader is not initialized",
		)
	}
	set, err := r.source.Corpora(ctx, corpusbase.CorporaID(id))
	if err != nil {
		return reportgeneration.CorpusSnapshot{}, err
	}
	return snapshot(set)
}

func snapshot(set corpusbase.Corpora) (reportgeneration.CorpusSnapshot, error) {
	restored, err := corpusbase.RestoreCorpora(set)
	if err != nil {
		return reportgeneration.CorpusSnapshot{}, fmt.Errorf("restore Community Corpora: %w", err)
	}
	seen := make(map[textunits.TextUnitID]struct{})
	textUnits := restored.TextUnits()
	ids := make([]string, 0, len(textUnits))
	for _, span := range textUnits {
		if _, found := seen[span.TextUnit.ID]; found {
			continue
		}
		seen[span.TextUnit.ID] = struct{}{}
		ids = append(ids, string(span.TextUnit.ID))
	}
	sort.Strings(ids)
	return reportgeneration.CorpusSnapshot{
		ID:          string(restored.ID),
		TextUnitIDs: ids,
	}, nil
}
