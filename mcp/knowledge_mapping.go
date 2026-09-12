package mcp

import (
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
)

func entityMemory(value EntityMemory) submission.Entity {
	return submission.Entity{
		Content: value.Content,
		Metadata: provenance.EntityMetadata{
			Frequency: value.Source.Frequency,
		},
	}
}

func relationMemory(value RelationMemory) submission.Relation {
	return submission.Relation{
		Content: value.Content,
		Metadata: provenance.RelationMetadata{
			Weight: value.Source.Weight,
		},
	}
}

func claimMemory(value ClaimMemory) (submission.Claim, error) {
	subject, err := subjectIdentity(value.Subject)
	if err != nil {
		return submission.Claim{}, err
	}
	return submission.Claim{
		Content: knowledge.ClaimContent{
			Subject:     subject,
			Type:        value.Type,
			Description: value.Description,
		},
		Metadata: provenance.ClaimMetadata{
			SubjectText: value.Source.SubjectText,
			ObjectText:  value.Source.ObjectText,
			Status:      value.Source.Status,
			StartDate:   value.Source.StartDate,
			EndDate:     value.Source.EndDate,
			SourceText:  value.Source.SourceText,
		},
	}, nil
}

func subjectIdentity(subject ClaimSubject) (knowledge.SubjectIdentity, error) {
	if subject.Entity != nil && subject.Relation == nil {
		return *subject.Entity, nil
	}
	if subject.Relation != nil && subject.Entity == nil {
		return *subject.Relation, nil
	}
	return nil, fmt.Errorf("%w: Claim subject must contain exactly one Entity or Relation", knowledge.ErrInvalidChange)
}

func entityValue(value knowledge.Entity) knowledge.Entity {
	value.Aliases = nonNil(append([]string(nil), value.Aliases...))
	return value
}

func claimValue(value knowledge.Claim) Claim {
	return Claim{
		ID:          string(value.ID),
		Subject:     subjectValue(value.Subject),
		Type:        value.Type,
		Description: value.Description,
	}
}

func subjectValue(subject knowledge.Subject) Subject {
	switch value := subject.(type) {
	case knowledge.EntityID:
		return Subject{
			Kind: "entity",
			ID:   string(value),
		}
	case knowledge.RelationID:
		return Subject{
			Kind: "relation",
			ID:   string(value),
		}
	default:
		return Subject{}
	}
}

func evidence(values []provenance.Evidence) []Evidence {
	result := make([]Evidence, len(values))
	for index, value := range values {
		result[index] = Evidence{
			ZoneID:     value.ZoneID,
			TextUnitID: value.TextUnitID,
			Source:     evidenceSource(value.Source),
		}
	}
	return result
}

func evidenceSource(source provenance.EvidenceSource) EvidenceSource {
	switch value := source.(type) {
	case provenance.CorporaSource:
		return EvidenceSource{
			Kind: "corpora",
			ID:   value.CorporaID,
		}
	case provenance.MessageSource:
		return EvidenceSource{
			Kind: "message",
			ID:   value.MessageID,
		}
	default:
		return EvidenceSource{}
	}
}

func entityConflict(value resolution.EntityConflict) EntityConflict {
	result := EntityConflict{
		BaseVersion: uint64(value.BaseVersion),
		Evidence:    entityEvidence(value.Confirmations),
		Alternatives: make(
			[]Alternative[knowledge.Entity, EntityEvidence],
			len(value.Candidates),
		),
	}
	if value.HasCurrent {
		current := entityVersion(value.Current)
		result.ID = string(value.Current.Knowledge.ID)
		result.Current = &current
	}
	for index, candidate := range value.Candidates {
		result.Alternatives[index] = Alternative[knowledge.Entity, EntityEvidence]{
			Revision: candidate.Candidate.Number,
			Content:  entityValue(candidate.Candidate.Content),
			Evidence: entityAlternativeEvidence(candidate.Sources),
			Reference: conflictReference(
				candidate.Candidate.Key.ContentHash,
				entitySourceIDs(candidate.Sources),
			),
		}
		if result.ID == "" {
			result.ID = string(candidate.Candidate.Key.ID)
		}
	}
	return result
}

func entityVersion(value knowledge.KnowledgeVersion[knowledge.Entity]) Version[knowledge.Entity] {
	return Version[knowledge.Entity]{
		Number:  uint64(value.Version),
		Deleted: value.Deleted,
		Content: entityValue(value.Knowledge),
	}
}

func relationVersion(value knowledge.KnowledgeVersion[knowledge.Relation]) Version[knowledge.Relation] {
	return Version[knowledge.Relation]{
		Number:  uint64(value.Version),
		Deleted: value.Deleted,
		Content: value.Knowledge,
	}
}

func claimVersion(value knowledge.KnowledgeVersion[knowledge.Claim]) Version[Claim] {
	return Version[Claim]{
		Number:  uint64(value.Version),
		Deleted: value.Deleted,
		Content: claimValue(value.Knowledge),
	}
}

func relationConflict(value resolution.RelationConflict) RelationConflict {
	result := RelationConflict{
		BaseVersion: uint64(value.BaseVersion),
		Evidence:    relationEvidence(value.Confirmations),
		Alternatives: make(
			[]Alternative[knowledge.Relation, RelationEvidence],
			len(value.Candidates),
		),
	}
	if value.HasCurrent {
		current := relationVersion(value.Current)
		result.ID = string(value.Current.Knowledge.ID)
		result.Current = &current
	}
	for index, candidate := range value.Candidates {
		result.Alternatives[index] = Alternative[knowledge.Relation, RelationEvidence]{
			Revision: candidate.Candidate.Number,
			Content:  candidate.Candidate.Content,
			Evidence: relationAlternativeEvidence(candidate.Sources),
			Reference: conflictReference(
				candidate.Candidate.Key.ContentHash,
				relationSourceIDs(candidate.Sources),
			),
		}
		if result.ID == "" {
			result.ID = string(candidate.Candidate.Key.ID)
		}
	}
	return result
}

func claimConflict(value resolution.ClaimConflict) ClaimConflict {
	result := ClaimConflict{
		BaseVersion: uint64(value.BaseVersion),
		Evidence:    claimEvidence(value.Confirmations),
		Alternatives: make(
			[]Alternative[Claim, ClaimEvidence],
			len(value.Candidates),
		),
	}
	if value.HasCurrent {
		current := claimVersion(value.Current)
		result.ID = string(value.Current.Knowledge.ID)
		result.Current = &current
	}
	for index, candidate := range value.Candidates {
		result.Alternatives[index] = Alternative[Claim, ClaimEvidence]{
			Revision: candidate.Candidate.Number,
			Content:  claimValue(candidate.Candidate.Content),
			Evidence: claimAlternativeEvidence(candidate.Sources),
			Reference: conflictReference(
				candidate.Candidate.Key.ContentHash,
				claimSourceIDs(candidate.Sources),
			),
		}
		if result.ID == "" {
			result.ID = string(candidate.Candidate.Key.ID)
		}
	}
	return result
}

func conflictReference(hash knowledge.Hash, sourceIDs []string) ConflictReference {
	return ConflictReference{
		ContentHash: hash.String(),
		SourceIDs:   nonNil(sourceIDs),
	}
}

func parseHash(value string) (knowledge.Hash, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != len(knowledge.Hash{}) {
		return knowledge.Hash{}, fmt.Errorf("%w: Conflict Content Hash is invalid", knowledge.ErrInvalidChange)
	}
	var hash knowledge.Hash
	copy(hash[:], decoded)
	return hash, nil
}

func conflictExpectations[ID knowledge.KnowledgeID](
	knowledgeID ID,
	baseVersion knowledge.Version,
	references []ConflictReference,
) ([]resolution.Expectation[ID], error) {
	result := make([]resolution.Expectation[ID], len(references))
	for index, reference := range references {
		hash, err := parseHash(reference.ContentHash)
		if err != nil {
			return nil, err
		}
		result[index] = resolution.Expectation[ID]{
			Candidate: candidate.Key[ID]{
				ID:          knowledgeID,
				BaseVersion: baseVersion,
				ContentHash: hash,
			},
			SourceIDs: reference.SourceIDs,
		}
	}
	return result, nil
}

func entityEvidence(values []provenance.EntityConfirmation) []EntityEvidence {
	result := make([]EntityEvidence, len(values))
	for index, value := range values {
		result[index] = EntityEvidence{
			SourceID:  value.SourceID,
			Frequency: value.Metadata.Frequency,
			Evidence:  evidence(value.Metadata.Evidence),
		}
	}
	return result
}

func relationEvidence(values []provenance.RelationConfirmation) []RelationEvidence {
	result := make([]RelationEvidence, len(values))
	for index, value := range values {
		result[index] = RelationEvidence{
			SourceID: value.SourceID,
			Weight:   value.Metadata.Weight,
			Evidence: evidence(value.Metadata.Evidence),
		}
	}
	return result
}

func claimEvidence(values []provenance.ClaimConfirmation) []ClaimEvidence {
	result := make([]ClaimEvidence, len(values))
	for index, value := range values {
		result[index] = ClaimEvidence{
			SourceID: value.SourceID,
			Records:  claimRecords(value.Metadata),
		}
	}
	return result
}

func entityAlternativeEvidence(values []provenance.EntitySubmission) []EntityEvidence {
	result := make([]EntityEvidence, len(values))
	for index, value := range values {
		result[index] = EntityEvidence{
			SourceID:  value.SourceID,
			Frequency: value.Metadata.Frequency,
			Evidence:  evidence(value.Metadata.Evidence),
		}
	}
	return result
}

func relationAlternativeEvidence(values []provenance.RelationSubmission) []RelationEvidence {
	result := make([]RelationEvidence, len(values))
	for index, value := range values {
		result[index] = RelationEvidence{
			SourceID: value.SourceID,
			Weight:   value.Metadata.Weight,
			Evidence: evidence(value.Metadata.Evidence),
		}
	}
	return result
}

func claimAlternativeEvidence(values []provenance.ClaimSubmission) []ClaimEvidence {
	result := make([]ClaimEvidence, len(values))
	for index, value := range values {
		result[index] = ClaimEvidence{
			SourceID: value.SourceID,
			Records:  claimRecords(value.Metadata),
		}
	}
	return result
}

func claimRecords(values []provenance.ClaimMetadata) []ClaimRecord {
	result := make([]ClaimRecord, len(values))
	for index, value := range values {
		result[index] = ClaimRecord{
			SubjectText: value.SubjectText,
			ObjectText:  value.ObjectText,
			Status:      value.Status,
			StartDate:   value.StartDate,
			EndDate:     value.EndDate,
			SourceText:  value.SourceText,
			Evidence:    evidence(value.Evidence),
		}
	}
	return result
}

func entitySourceIDs(values []provenance.EntitySubmission) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.SourceID
	}
	return canonicalIDs(result)
}

func relationSourceIDs(values []provenance.RelationSubmission) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.SourceID
	}
	return canonicalIDs(result)
}

func claimSourceIDs(values []provenance.ClaimSubmission) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.SourceID
	}
	return canonicalIDs(result)
}

func canonicalIDs(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
