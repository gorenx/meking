// Package submission accepts one Source's reference-closed Knowledge content,
// confirms identical formal content, and preserves different content as
// Conflict Candidates. Missing content never means deletion.
package submission

import (
	"context"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/transaction"
)

type Entity struct {
	Content  knowledge.EntityContent
	Metadata provenance.EntityMetadata
}

type Relation struct {
	Content  knowledge.RelationContent
	Metadata provenance.RelationMetadata
}

// Claim is one parser-produced Claim record. Callers must not group records
// from the same TextUnit into a nested collection.
type Claim struct {
	Content  knowledge.ClaimContent
	Metadata provenance.ClaimMetadata
}

type Command struct {
	Source    provenance.Source
	Entities  []Entity
	Relations []Relation
	Claims    []Claim
}

type CandidateSet struct {
	Entities  []candidate.Key[knowledge.EntityID]
	Relations []candidate.Key[knowledge.RelationID]
	Claims    []candidate.Key[knowledge.ClaimID]
}

type Result struct {
	SourceID          string
	CreatedVersions   knowledge.References
	ConfirmedVersions knowledge.References
	OpenedCandidates  CandidateSet
	ReusedCandidates  CandidateSet
}

// KnowledgeIdentities owns the stable binding between canonical identities and
// permanent Knowledge IDs.
type KnowledgeIdentities interface {
	EntityID(context.Context, knowledge.EntityIdentity) (knowledge.EntityID, bool, error)
	RelationID(context.Context, knowledge.RelationKey) (knowledge.RelationID, bool, error)
	ClaimID(context.Context, knowledge.ClaimIdentity) (knowledge.ClaimID, bool, error)

	ReserveEntity(context.Context, knowledge.EntityID, knowledge.EntityIdentity) error
	ReserveRelation(context.Context, knowledge.RelationID, knowledge.RelationKey) error
	ReserveClaim(context.Context, knowledge.ClaimID, knowledge.ClaimIdentity) error
}

type Sources interface {
	Source(context.Context, string) (provenance.Source, bool, error)
	RecordSource(context.Context, provenance.Source, []provenance.Evidence) error
	SubmissionResult(context.Context, string) (Result, error)

	ConfirmEntity(context.Context, provenance.EntityConfirmation) error
	ConfirmRelation(context.Context, provenance.RelationConfirmation) error
	ConfirmClaim(context.Context, provenance.ClaimConfirmation) error

	SubmitEntity(context.Context, provenance.EntitySubmission) error
	SubmitRelation(context.Context, provenance.RelationSubmission) error
	SubmitClaim(context.Context, provenance.ClaimSubmission) error
}

type Candidates interface {
	FindEntity(context.Context, candidate.Key[knowledge.EntityID]) (candidate.Entity, bool, error)
	FindRelation(context.Context, candidate.Key[knowledge.RelationID]) (candidate.Relation, bool, error)
	FindClaim(context.Context, candidate.Key[knowledge.ClaimID]) (candidate.Claim, bool, error)

	NextEntityNumber(context.Context, knowledge.EntityID, knowledge.Version) (uint64, error)
	NextRelationNumber(context.Context, knowledge.RelationID, knowledge.Version) (uint64, error)
	NextClaimNumber(context.Context, knowledge.ClaimID, knowledge.Version) (uint64, error)

	AddEntity(context.Context, candidate.Entity, string) error
	AddRelation(context.Context, candidate.Relation, string) error
	AddClaim(context.Context, candidate.Claim, string) error
}

type EvidenceVerifier interface {
	VerifyEvidence(ctx context.Context, evidence []provenance.Evidence) error
}

type Dependencies struct {
	Tx                  transaction.Tx
	KnowledgeIdentities KnowledgeIdentities
	VersionHistory      knowledge.VersionHistory
	Sources             Sources
	Candidates          Candidates
	EvidenceVerifier    EvidenceVerifier
}

type Application struct {
	Dependencies
}
