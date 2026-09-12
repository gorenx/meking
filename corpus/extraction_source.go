package corpus

import (
	"context"
	"fmt"
	"math"
)

// ExtractionSource is the immutable Corpus boundary offered to a Knowledge
// Extraction consumer. It exposes only the identity and integrity facts that
// the consumer must fix when accepting a Corpora target.
type ExtractionSource struct {
	CorporaID         string
	TextUnitSetDigest string
	TextUnitCount     uint64
}

func (service *Service) ExtractionSource(
	ctx context.Context,
	target uint64,
) (ExtractionSource, error) {
	if service == nil || target == 0 || target > math.MaxInt64 {
		return ExtractionSource{}, fmt.Errorf("%w: Extraction target is invalid", ErrInvalidCorpus)
	}
	corporaID, err := NewCorporaID(int64(target))
	if err != nil {
		return ExtractionSource{}, err
	}
	set, err := service.Corpora(ctx, corporaID)
	if err != nil {
		return ExtractionSource{}, err
	}
	return ExtractionSource{
		CorporaID:         FormatCorporaID(set.ID),
		TextUnitSetDigest: set.TextUnitSetDigest(),
		TextUnitCount:     uint64(len(set.TextUnits())),
	}, nil
}
