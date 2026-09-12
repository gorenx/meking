// Package zonemerger submits formal Child Zone Knowledge to its direct Parent.
// When Parent formal content differs, it creates a candidate in the Child so
// the Child owns review and resolution.
package zonemerger

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/zone"
)

var (
	ErrInvalidMerge    = errors.New("invalid Child Knowledge merge")
	ErrConflictChanged = errors.New("Zone Knowledge conflict changed after assignment")
)

// Provenance exposes the source metadata attached to one exact formal Version.
// The merger forwards that metadata with Child content so Parent confirmation
// never loses the original Corpus evidence.
type Provenance interface {
	Entity(
		context.Context,
		knowledge.Reference[knowledge.EntityID],
	) (provenance.EntityMetadata, error)
	Relation(
		context.Context,
		knowledge.Reference[knowledge.RelationID],
	) (provenance.RelationMetadata, error)
	Claim(
		context.Context,
		knowledge.Reference[knowledge.ClaimID],
	) ([]provenance.ClaimMetadata, error)
}

type Identities interface {
	EntityID(ctx context.Context, identity knowledge.EntityIdentity) (knowledge.EntityID, bool, error)
	RelationID(ctx context.Context, key knowledge.RelationKey) (knowledge.RelationID, bool, error)
	ClaimID(ctx context.Context, identity knowledge.ClaimIdentity) (knowledge.ClaimID, bool, error)
	EntityIdentity(ctx context.Context, entityID knowledge.EntityID) (knowledge.EntityIdentity, bool, error)
	RelationKey(ctx context.Context, relationID knowledge.RelationID) (knowledge.RelationKey, bool, error)
}

// EntityConflict identifies one Child Entity conflict caused by differing
// Parent formal content. Its natural identity is the two stable Entity IDs and
// their formal Versions.
type EntityConflict struct {
	ChildZoneID      zone.ID
	ChildEntityID    knowledge.EntityID
	ChildBase        knowledge.Version
	ParentZoneID     zone.ID
	ParentEntityID   knowledge.EntityID
	ParentVersion    knowledge.Version
	CandidateHash    knowledge.Hash
	ParentReconciled bool
}

// RelationConflict identifies one Child Relation conflict caused by differing
// Parent formal content.
type RelationConflict struct {
	ChildZoneID      zone.ID
	ChildRelationID  knowledge.RelationID
	ChildBase        knowledge.Version
	ParentZoneID     zone.ID
	ParentRelationID knowledge.RelationID
	ParentVersion    knowledge.Version
	CandidateHash    knowledge.Hash
	ParentReconciled bool
}

// ClaimConflict identifies one Child Claim conflict caused by differing Parent
// formal content.
type ClaimConflict struct {
	ChildZoneID      zone.ID
	ChildClaimID     knowledge.ClaimID
	ChildBase        knowledge.Version
	ParentZoneID     zone.ID
	ParentClaimID    knowledge.ClaimID
	ParentVersion    knowledge.Version
	CandidateHash    knowledge.Hash
	ParentReconciled bool
}

// Conflicts persists the natural cross-Zone conflict assignment. It does not
// allocate a separate Conflict ID.
type Conflicts interface {
	SaveEntityConflict(context.Context, EntityConflict) error
	SaveRelationConflict(context.Context, RelationConflict) error
	SaveClaimConflict(context.Context, ClaimConflict) error

	EntityConflict(context.Context, knowledge.EntityID, knowledge.Version) (EntityConflict, bool, error)
	RelationConflict(context.Context, knowledge.RelationID, knowledge.Version) (RelationConflict, bool, error)
	ClaimConflict(context.Context, knowledge.ClaimID, knowledge.Version) (ClaimConflict, bool, error)

	ResolvedEntityConflict(context.Context, string) (EntityConflict, error)
	ResolvedRelationConflict(context.Context, string) (RelationConflict, error)
	ResolvedClaimConflict(context.Context, string) (ClaimConflict, error)

	ResolveEntityConflict(context.Context, EntityConflict, string) error
	ResolveRelationConflict(context.Context, RelationConflict, string) error
	ResolveClaimConflict(context.Context, ClaimConflict, string) error

	ReconcileEntityConflict(context.Context, EntityConflict, string) error
	ReconcileRelationConflict(context.Context, RelationConflict, string) error
	ReconcileClaimConflict(context.Context, ClaimConflict, string) error
}

type EntityReview struct {
	Assignment EntityConflict
	Child      resolution.EntityConflict
	Parent     knowledge.KnowledgeVersion[knowledge.Entity]
}

type RelationReview struct {
	Assignment RelationConflict
	Child      resolution.RelationConflict
	Parent     knowledge.KnowledgeVersion[knowledge.Relation]
}

type ClaimReview struct {
	Assignment ClaimConflict
	Child      resolution.ClaimConflict
	Parent     knowledge.KnowledgeVersion[knowledge.Claim]
}
