package epoch

import (
	"context"

	"github.com/memoria-space/meking/knowledge"
)

// Readiness proves that the exact immutable cross-context results supplied to
// Publish are complete through PublicationTarget. Implementations must use exact-ID reads
// and must not select another domain's Current as a substitute.
type Readiness interface {
	Check(
		ctx context.Context,
		structureID StructureID,
		corporaID CorporaID,
		versions knowledge.Manifest,
	) error
}
