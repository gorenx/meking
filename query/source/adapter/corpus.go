// Package adapter maps provider-owned Corpus application values into the
// source projection shared by Query aggregates.
package adapter

import (
	"context"
	"errors"
	"github.com/memoria-space/meking/corpus/textunits"

	"github.com/memoria-space/meking/corpus"
	querysource "github.com/memoria-space/meking/query/source"
)

// Corpus is the exact, batched provider capability consumed by Query source
// projection. The interface is local to this adapter so Corpus does not depend
// on Query DTOs.
type Corpus interface {
	TextUnitLocations(
		ctx context.Context,
		setID corpus.CorporaID,
		ids []textunits.TextUnitID,
	) ([]corpus.TextUnitLocation, error)
}

// SourceReader maps exact Corpus occurrences without selecting, sorting, or
// deduplicating evidence after the provider has fixed Corpus order.
type SourceReader struct {
	// corpus is the fact source for TextUnit content and Document positions.
	corpus Corpus
}

var _ querysource.Reader = (*SourceReader)(nil)

// NewSourceReader creates the Query source adapter from Corpus's batch API.
func NewSourceReader(corpusReader Corpus) (*SourceReader, error) {
	if corpusReader == nil {
		return nil, errors.New("create Query SourceReader: Corpus is required")
	}
	return &SourceReader{corpus: corpusReader}, nil
}

// Read performs one batch call and copies provider values into Query-owned
// occurrences. Validation of Corpus identities and membership stays in Corpus.
func (r *SourceReader) Read(
	ctx context.Context,
	corporaID string,
	textUnitIDs []string,
) ([]querysource.TextUnitSource, error) {
	if r == nil || r.corpus == nil {
		return nil, errors.New("Query SourceReader is not configured")
	}
	ids := make([]textunits.TextUnitID, len(textUnitIDs))
	for index, id := range textUnitIDs {
		ids[index] = textunits.TextUnitID(id)
	}
	occurrences, err := r.corpus.TextUnitLocations(
		ctx,
		corpus.CorporaID(corporaID),
		ids,
	)
	if err != nil {
		return nil, err
	}
	result := make([]querysource.TextUnitSource, len(occurrences))
	for index, occurrence := range occurrences {
		result[index] = querysource.TextUnitSource{
			CorporaID:        string(occurrence.CorporaID),
			TextUnitID:       string(occurrence.TextUnit.TextUnit.ID),
			Text:             occurrence.TextUnit.TextUnit.Text,
			TextID:           string(occurrence.TextID),
			DocumentID:       string(occurrence.DocumentID),
			DocumentLocation: string(occurrence.DocumentLocation),
			TextTitle:        occurrence.TextTitle,
		}
	}
	return result, nil
}
