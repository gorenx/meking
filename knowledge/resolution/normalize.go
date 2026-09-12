package resolution

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
)

type entityResolution struct {
	source      provenance.Source
	baseVersion knowledge.Version
	final       knowledge.Entity
	candidates  []EntityExpectation
}

type relationResolution struct {
	source      provenance.Source
	baseVersion knowledge.Version
	final       knowledge.Relation
	candidates  []RelationExpectation
}

type claimResolution struct {
	source      provenance.Source
	baseVersion knowledge.Version
	final       knowledge.Claim
	candidates  []ClaimExpectation
}

func normalizeEntity(command EntityCommand) (entityResolution, error) {
	if err := validateResolutionSource(command.Source); err != nil {
		return entityResolution{}, err
	}
	if err := knowledge.ValidateVersion(command.BaseVersion); err != nil {
		return entityResolution{}, err
	}
	if err := knowledge.ValidateEntity(command.Final); err != nil {
		return entityResolution{}, err
	}
	candidates, err := canonicalExpectations("Entity", command.Final.ID, command.BaseVersion, command.Candidates)
	if err != nil {
		return entityResolution{}, err
	}
	result := entityResolution{
		source: command.Source, baseVersion: command.BaseVersion,
		final: command.Final, candidates: candidates,
	}
	result.final.Aliases = append([]string(nil), command.Final.Aliases...)
	return result, nil
}

func normalizeRelation(command RelationCommand) (relationResolution, error) {
	if err := validateResolutionSource(command.Source); err != nil {
		return relationResolution{}, err
	}
	if err := knowledge.ValidateVersion(command.BaseVersion); err != nil {
		return relationResolution{}, err
	}
	if err := knowledge.ValidateRelation(command.Final); err != nil {
		return relationResolution{}, err
	}
	candidates, err := canonicalExpectations("Relation", command.Final.ID, command.BaseVersion, command.Candidates)
	if err != nil {
		return relationResolution{}, err
	}
	result := relationResolution{
		source: command.Source, baseVersion: command.BaseVersion,
		final: command.Final, candidates: candidates,
	}
	return result, nil
}

func normalizeClaim(command ClaimCommand) (claimResolution, error) {
	if err := validateResolutionSource(command.Source); err != nil {
		return claimResolution{}, err
	}
	if err := knowledge.ValidateVersion(command.BaseVersion); err != nil {
		return claimResolution{}, err
	}
	if err := knowledge.ValidateClaim(command.Final); err != nil {
		return claimResolution{}, err
	}
	candidates, err := canonicalExpectations("Claim", command.Final.ID, command.BaseVersion, command.Candidates)
	if err != nil {
		return claimResolution{}, err
	}
	result := claimResolution{
		source: command.Source, baseVersion: command.BaseVersion,
		final: command.Final, candidates: candidates,
	}
	return result, nil
}

func validateResolutionSource(source provenance.Source) error {
	if err := provenance.Validate(source); err != nil {
		return err
	}
	if source.Kind != provenance.Resolution {
		return fmt.Errorf("%w: Conflict Resolution requires a Resolution Source", provenance.ErrInvalidSource)
	}
	return nil
}

func canonicalExpectations[ID knowledge.KnowledgeID](
	objectName string,
	id ID,
	base knowledge.Version,
	values []Expectation[ID],
) ([]Expectation[ID], error) {
	result := append([]Expectation[ID](nil), values...)
	for index := range result {
		if result[index].Candidate.ID != id || result[index].Candidate.BaseVersion != base {
			return nil, fmt.Errorf(
				"%w: %s Resolution Candidate does not match its target",
				knowledge.ErrInvalidChange,
				objectName,
			)
		}
		if err := canonicalSourceIDs(&result[index].SourceIDs); err != nil {
			return nil, err
		}
	}
	slices.SortFunc(result, func(left, right Expectation[ID]) int {
		return bytes.Compare(left.Candidate.ContentHash[:], right.Candidate.ContentHash[:])
	})
	if len(result) == 0 {
		return nil, fmt.Errorf(
			"%w: %s Resolution requires pending Candidates",
			knowledge.ErrInvalidChange,
			objectName,
		)
	}
	for index := 1; index < len(result); index++ {
		if result[index-1].Candidate == result[index].Candidate {
			return nil, fmt.Errorf(
				"%w: %s Resolution contains a duplicate Candidate",
				knowledge.ErrInvalidChange,
				objectName,
			)
		}
	}
	return result, nil
}

func canonicalSourceIDs(values *[]string) error {
	for _, value := range *values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: Candidate Source ID is required", knowledge.ErrInvalidChange)
		}
	}
	slices.Sort(*values)
	*values = slices.Compact(*values)
	if len(*values) == 0 {
		return fmt.Errorf("%w: Candidate must include pending Sources", knowledge.ErrInvalidChange)
	}
	return nil
}

func actualEntityExpectations(value EntityConflict) []EntityExpectation {
	result := make([]EntityExpectation, 0, len(value.Candidates))
	for _, candidate := range value.Candidates {
		sources := pendingEntitySources(candidate.Sources)
		if len(sources) > 0 {
			result = append(result, EntityExpectation{Candidate: candidate.Candidate.Key, SourceIDs: sources})
		}
	}
	result, _ = canonicalExpectations("Entity", value.Candidates[0].Candidate.Key.ID, value.BaseVersion, result)
	return result
}

func actualRelationExpectations(value RelationConflict) []RelationExpectation {
	result := make([]RelationExpectation, 0, len(value.Candidates))
	for _, candidate := range value.Candidates {
		sources := pendingRelationSources(candidate.Sources)
		if len(sources) > 0 {
			result = append(result, RelationExpectation{Candidate: candidate.Candidate.Key, SourceIDs: sources})
		}
	}
	result, _ = canonicalExpectations("Relation", value.Candidates[0].Candidate.Key.ID, value.BaseVersion, result)
	return result
}

func actualClaimExpectations(value ClaimConflict) []ClaimExpectation {
	result := make([]ClaimExpectation, 0, len(value.Candidates))
	for _, candidate := range value.Candidates {
		sources := pendingClaimSources(candidate.Sources)
		if len(sources) > 0 {
			result = append(result, ClaimExpectation{Candidate: candidate.Candidate.Key, SourceIDs: sources})
		}
	}
	result, _ = canonicalExpectations("Claim", value.Candidates[0].Candidate.Key.ID, value.BaseVersion, result)
	return result
}

func pendingEntitySources(values []provenance.EntitySubmission) []string {
	var result []string
	for _, value := range values {
		result = append(result, value.SourceID)
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func pendingRelationSources(values []provenance.RelationSubmission) []string {
	var result []string
	for _, value := range values {
		result = append(result, value.SourceID)
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func pendingClaimSources(values []provenance.ClaimSubmission) []string {
	var result []string
	for _, value := range values {
		result = append(result, value.SourceID)
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func equalExpectations[ID knowledge.KnowledgeID](left, right []Expectation[ID]) bool {
	return slices.EqualFunc(left, right, func(a, b Expectation[ID]) bool {
		return a.Candidate == b.Candidate && slices.Equal(a.SourceIDs, b.SourceIDs)
	})
}

func candidateEntity(key candidate.Key[knowledge.EntityID], values []EntityCandidate) (EntityCandidate, bool) {
	for _, value := range values {
		if value.Candidate.Key == key {
			return value, true
		}
	}
	return EntityCandidate{}, false
}

func candidateRelation(key candidate.Key[knowledge.RelationID], values []RelationCandidate) (RelationCandidate, bool) {
	for _, value := range values {
		if value.Candidate.Key == key {
			return value, true
		}
	}
	return RelationCandidate{}, false
}

func candidateClaim(key candidate.Key[knowledge.ClaimID], values []ClaimCandidate) (ClaimCandidate, bool) {
	for _, value := range values {
		if value.Candidate.Key == key {
			return value, true
		}
	}
	return ClaimCandidate{}, false
}
