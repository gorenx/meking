package epoch

import (
	"context"
	"time"
)

// ReadStore is the Epoch-owned persistence port for Current and exact immutable
// reads. Query compositions depend on this narrower capability and do not need
// Knowledge-change or publication-readiness collaborators.
type ReadStore interface {
	Current(ctx context.Context) (Epoch, error)
	Load(ctx context.Context, id ID) (Epoch, error)
	LoadStructure(ctx context.Context, id StructureID) (Epoch, error)
}

// Store adds Epoch publication to ReadStore. Publish must create one immutable
// row and switch Current in the same transaction after comparing ExpectedEpoch.
type Store interface {
	ReadStore
	Publish(
		ctx context.Context,
		target PublicationTarget,
		corporaID CorporaID,
		publishedAt time.Time,
	) (Epoch, error)
}
