package query

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/knowledge"
)

var (
	// ErrNoEpoch means no complete publication is available to a new
	// query request. Knowledge-only reads remain independently available.
	ErrNoEpoch = errors.New("no query epoch")
	// ErrEpochNotFound means a caller requested an exact immutable Epoch that
	// does not exist in this Zone.
	ErrEpochNotFound = errors.New("query epoch not found")
)

// Epoch is the minimal unified publication fixed once by every query.
// It contains only identities consumed by Query; Corpus, Report, and Knowledge
// content remains in the owning contexts and is read lazily by exact identity.
type Epoch struct {
	// ID is the positive Zone-local Epoch sequence returned to callers as
	// the unified version of this query result.
	ID int64
	// Knowledge is the exact formal Version set consumed by the published
	// derived results.
	Knowledge knowledge.Manifest
	// CorporaID is the immutable Corpus evidence and TextUnit vector identity
	// selected by this Epoch.
	CorporaID string
	// StructureID identifies the immutable Community Structure that belongs to
	// this Epoch's Corpus and Knowledge boundary.
	StructureID string
	// ReportSetID is the immutable Community Report selection and optional report
	// vector identity resolved for StructureID. It is empty while reports lag.
	ReportSetID string
}

func (epoch Epoch) Equal(other Epoch) bool {
	return epoch.ID == other.ID &&
		epoch.Knowledge.Equal(other.Knowledge) &&
		epoch.CorporaID == other.CorporaID &&
		epoch.StructureID == other.StructureID &&
		epoch.ReportSetID == other.ReportSetID
}

// EpochReader fixes the Current Epoch at the start of one Query use case.
// Implementations return an isolated value and never resolve Corpus or Report
// Current selections on Query's behalf.
type EpochReader interface {
	Current(ctx context.Context) (Epoch, error)
}
