// Package resolution exposes the Agent-facing conflict read and decision
// boundary. It never invokes a model and accepts only a complete final value.
package resolution

import (
	"context"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/transaction"
)

type EntityCandidate struct {
	Candidate candidate.Entity
	Sources   []provenance.EntitySubmission
}

type RelationCandidate struct {
	Candidate candidate.Relation
	Sources   []provenance.RelationSubmission
}

type ClaimCandidate struct {
	Candidate candidate.Claim
	Sources   []provenance.ClaimSubmission
}

type EntityConflict struct {
	BaseVersion   knowledge.Version
	Current       knowledge.KnowledgeVersion[knowledge.Entity]
	HasCurrent    bool
	Confirmations []provenance.EntityConfirmation
	Candidates    []EntityCandidate
}

type RelationConflict struct {
	BaseVersion   knowledge.Version
	Current       knowledge.KnowledgeVersion[knowledge.Relation]
	HasCurrent    bool
	Confirmations []provenance.RelationConfirmation
	Candidates    []RelationCandidate
}

type ClaimConflict struct {
	BaseVersion   knowledge.Version
	Current       knowledge.KnowledgeVersion[knowledge.Claim]
	HasCurrent    bool
	Confirmations []provenance.ClaimConfirmation
	Candidates    []ClaimCandidate
}

type Expectation[ID knowledge.KnowledgeID] struct {
	Candidate candidate.Key[ID]
	SourceIDs []string
}

type EntityExpectation = Expectation[knowledge.EntityID]
type RelationExpectation = Expectation[knowledge.RelationID]
type ClaimExpectation = Expectation[knowledge.ClaimID]

type EntityCommand struct {
	Source      provenance.Source
	BaseVersion knowledge.Version
	Final       knowledge.Entity
	Candidates  []EntityExpectation
}

type RelationCommand struct {
	Source      provenance.Source
	BaseVersion knowledge.Version
	Final       knowledge.Relation
	Candidates  []RelationExpectation
}

type ClaimCommand struct {
	Source      provenance.Source
	BaseVersion knowledge.Version
	Final       knowledge.Claim
	Candidates  []ClaimExpectation
}

type Result struct {
	Version        knowledge.Version
	CreatedVersion bool
}

const (
	DefaultPageSize = 50
	MaximumPageSize = 200
)

type ConflictPageRequest struct {
	After string
	Limit int
}

type EntityConflictPage = knowledge.Page[EntityConflict, knowledge.EntityID]
type RelationConflictPage = knowledge.Page[RelationConflict, knowledge.RelationID]
type ClaimConflictPage = knowledge.Page[ClaimConflict, knowledge.ClaimID]

type KnowledgeIdentities interface {
	EntityIdentity(context.Context, knowledge.EntityID) (knowledge.EntityIdentity, bool, error)
	RelationKey(context.Context, knowledge.RelationID) (knowledge.RelationKey, bool, error)
	ClaimIdentity(context.Context, knowledge.ClaimID) (knowledge.ClaimIdentity, bool, error)
}

type Candidates interface {
	ListEntities(ctx context.Context, entityID knowledge.EntityID, baseVersion knowledge.Version) ([]candidate.Entity, error)
	ListRelations(ctx context.Context, relationID knowledge.RelationID, baseVersion knowledge.Version) ([]candidate.Relation, error)
	ListClaims(ctx context.Context, claimID knowledge.ClaimID, baseVersion knowledge.Version) ([]candidate.Claim, error)

	ListEntityIDs(ctx context.Context, afterEntityID knowledge.EntityID, limit int) ([]knowledge.EntityID, error)
	ListRelationIDs(ctx context.Context, afterRelationID knowledge.RelationID, limit int) ([]knowledge.RelationID, error)
	ListClaimIDs(ctx context.Context, afterClaimID knowledge.ClaimID, limit int) ([]knowledge.ClaimID, error)
}

// Provenance is the source-fact port used by Conflict Resolution. It reads the
// Sources supporting the current formal Version and pending Candidates, records
// the Resolution Source, attributes the chosen formal Version to that Source,
// and marks the reviewed Candidates as resolved. Conflict decisions remain in
// Application; implementations only persist and restore these facts.
type Provenance interface {
	provenance.EvidenceReader

	Source(ctx context.Context, sourceID string) (provenance.Source, bool, error)
	RecordSource(ctx context.Context, source provenance.Source, evidence []provenance.Evidence) error
	ResolutionResult(ctx context.Context, sourceID string) (Result, error)

	EntitySubmissions(ctx context.Context, candidate candidate.Key[knowledge.EntityID]) ([]provenance.EntitySubmission, error)
	RelationSubmissions(ctx context.Context, candidate candidate.Key[knowledge.RelationID]) ([]provenance.RelationSubmission, error)
	ClaimSubmissions(ctx context.Context, candidate candidate.Key[knowledge.ClaimID]) ([]provenance.ClaimSubmission, error)

	ConfirmEntity(ctx context.Context, confirmation provenance.EntityConfirmation) error
	ConfirmRelation(ctx context.Context, confirmation provenance.RelationConfirmation) error
	ConfirmClaim(ctx context.Context, confirmation provenance.ClaimConfirmation) error

	ResolveEntityCandidate(ctx context.Context, resolution provenance.EntityCandidateResolution) error
	ResolveRelationCandidate(ctx context.Context, resolution provenance.RelationCandidateResolution) error
	ResolveClaimCandidate(ctx context.Context, resolution provenance.ClaimCandidateResolution) error
}

type Dependencies struct {
	Tx                  transaction.Tx
	KnowledgeIdentities KnowledgeIdentities
	VersionHistory      knowledge.VersionHistory
	Candidates          Candidates
	Provenance          Provenance
}

type Application struct {
	Dependencies
}
