package graph

import (
	"context"
)

type KnowledgeEntity struct {
	ID            string
	Version       uint64
	Deleted       bool
	Title         string
	Type          string
	Aliases       []string
	Description   string
	Degree        int
	TextUnitCount int
}

type KnowledgeRelation struct {
	ID             string
	Version        uint64
	Deleted        bool
	SourceEntityID string
	TargetEntityID string
	Type           string
	Description    string
	Weight         float64
	CombinedDegree int
	TextUnitCount  int
}

// KnowledgePage preserves the continuation selected by the fixed Knowledge
// read view. The graph use case must not infer completion from the item count.
type KnowledgePage[T any] struct {
	Items     []T
	NextAfter string
	HasMore   bool
}

// KnowledgeView remains fixed at one exact Knowledge version set until Close. Its pages
// include deleted current versions so the graph use case, rather than the
// provider adapter, owns visibility filtering.
type KnowledgeView interface {
	Entities(ctx context.Context, after string, limit int) (KnowledgePage[KnowledgeEntity], error)
	Relations(ctx context.Context, after string, limit int) (KnowledgePage[KnowledgeRelation], error)
	Close() error
}

type KnowledgeReader interface {
	OpenCurrent(ctx context.Context) (KnowledgeView, error)
}

type Membership struct {
	ID        string
	Number    int
	Level     int
	ParentID  *string
	EntityIDs []string
}

type Structure struct {
	ID             string
	CommunitySetID string
	CorporaID      string
	Memberships    []Membership
}

type StructureReader interface {
	Current(ctx context.Context) (Structure, error)
}
