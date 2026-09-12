package vectorindex

import (
	"context"

	"github.com/memoria-space/meking/knowledge"
)

// Target is the exact active Entity Version membership of one vector
// Namespace. Relation and Claim Versions do not affect Entity vector content.
type Target struct {
	Entities []knowledge.Reference[knowledge.EntityID]
}

type Builder interface {
	Build(context.Context, Target) (int, error)
}
