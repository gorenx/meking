package epoch

import (
	"time"

	"github.com/memoria-space/meking/knowledge"
)

// ID is the positive, Zone-local sequence assigned only when one complete
// A publication becomes Current. Zero is reserved for the absence of a
// published Epoch and is never persisted as an Epoch identity.
type ID int64

// CorporaID identifies the immutable Corpora used by Basic retrieval and by
// the ReportSet in the same Epoch. Epoch keeps the provider-owned identity
// opaque and validates the referenced Corpus before publication.
type CorporaID string

// StructureID identifies the immutable Community Structure bound by this Epoch.
type StructureID string

// Epoch is one immutable, query-visible publication binding. Query fixes its ID
// once, then lazily reads the referenced facts from their owning contexts.
type Epoch struct {
	// ID is a positive sequence allocated by Epoch persistence during successful
	// publication. It is the only ordering identity for unified Query versions.
	ID ID
	// Knowledge is the exact formal Version set verified before this Epoch became
	// Current.
	Knowledge knowledge.Manifest
	// CorporaID is the single immutable Corpora used by this publication.
	CorporaID CorporaID
	// StructureID identifies the Community hierarchy derived from the same
	// CorporaID and Knowledge Version set.
	StructureID StructureID
	// PublishedAt is the non-zero UTC completion time recorded after publication
	// readiness succeeds. It supports diagnostics and retention; it never
	// determines Epoch order.
	PublishedAt time.Time
}

func (epoch Epoch) Equal(other Epoch) bool {
	return epoch.ID == other.ID &&
		epoch.Knowledge.Equal(other.Knowledge) &&
		epoch.CorporaID == other.CorporaID &&
		epoch.StructureID == other.StructureID &&
		epoch.PublishedAt.Equal(other.PublishedAt)
}

// PublicationTarget fixes the optimistic Current baseline and exact Knowledge
// Versions verified by one recoverable Publication.
type PublicationTarget struct {
	// ExpectedEpoch is the Current Epoch observed before reading Knowledge. Zero
	// is the initial-publication sentinel; positive values participate in CAS.
	ExpectedEpoch ID
	// Knowledge is fixed for this run. Later Versions require another Publication.
	Knowledge   knowledge.Manifest
	StructureID StructureID
}
