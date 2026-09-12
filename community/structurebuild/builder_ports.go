package structurebuild

import (
	"context"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/knowledge"
)

type Entity struct {
	ID      string
	Version uint64
}

type Relation struct {
	ID             string
	Version        uint64
	SourceEntityID string
	TargetEntityID string
	Weight         float64
}

type KnowledgeSnapshot struct {
	Entities  []Entity
	Relations []Relation
}

type KnowledgeReader interface {
	Snapshot(context.Context, knowledge.Manifest) (KnowledgeSnapshot, error)
}

type CommunitySets interface {
	Save(context.Context, community.CommunitySet) error
	Load(context.Context, community.CommunitySetID) (community.CommunitySet, error)
}

type Structures interface {
	community.StructureStore
	community.StructureStateStore
}

type Producer interface {
	PublishStructure(context.Context, Prepared) error
}

type Prepared struct {
	SourceEventID string
	CorrelationID string
	Structure     community.Structure
}
