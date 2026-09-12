package knowledge

import "context"

type CurrentVersions interface {
	CurrentEntity(ctx context.Context, entityID EntityID) (KnowledgeVersion[Entity], bool, error)
	CurrentRelation(ctx context.Context, relationID RelationID) (KnowledgeVersion[Relation], bool, error)
	CurrentClaim(ctx context.Context, claimID ClaimID) (KnowledgeVersion[Claim], bool, error)
}

// VersionHistory is the mutation boundary for immutable formal Knowledge Versions.
// Applications own the rules deciding when a Version may be appended.
type VersionHistory interface {
	CurrentVersions

	AppendEntityVersion(ctx context.Context, version KnowledgeVersion[Entity], sourceID string) error
	AppendRelationVersion(ctx context.Context, version KnowledgeVersion[Relation], sourceID string) error
	AppendClaimVersion(ctx context.Context, version KnowledgeVersion[Claim], sourceID string) error
}
