package provenance

import (
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
)

// EntityConfirmation attributes one formal Entity Version to a Source.
type EntityConfirmation struct {
	EntityID knowledge.EntityID
	Version  knowledge.Version
	SourceID string
	Metadata EntityMetadata
}

// RelationConfirmation attributes one formal Relation Version to a Source.
type RelationConfirmation struct {
	RelationID knowledge.RelationID
	Version    knowledge.Version
	SourceID   string
	Metadata   RelationMetadata
}

// ClaimConfirmation attributes one formal Claim Version to a Source.
type ClaimConfirmation struct {
	ClaimID  knowledge.ClaimID
	Version  knowledge.Version
	SourceID string
	Metadata []ClaimMetadata
}

// EntitySubmission records one Source's support for a Candidate.
type EntitySubmission struct {
	Candidate       candidate.Key[knowledge.EntityID]
	SourceID        string
	OpenedCandidate bool
	Metadata        EntityMetadata
}

// RelationSubmission records one Source's support for a Candidate.
type RelationSubmission struct {
	Candidate       candidate.Key[knowledge.RelationID]
	SourceID        string
	OpenedCandidate bool
	Metadata        RelationMetadata
}

// ClaimSubmission records one Source's support for a Candidate.
type ClaimSubmission struct {
	Candidate       candidate.Key[knowledge.ClaimID]
	SourceID        string
	OpenedCandidate bool
	Metadata        []ClaimMetadata
}

type EntityCandidateResolution struct {
	Candidate candidate.Key[knowledge.EntityID]
	SourceID  string
}

type RelationCandidateResolution struct {
	Candidate candidate.Key[knowledge.RelationID]
	SourceID  string
}

type ClaimCandidateResolution struct {
	Candidate candidate.Key[knowledge.ClaimID]
	SourceID  string
}
