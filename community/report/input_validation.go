package report

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/internal/uuid"
)

// validateReportInput checks the Community-owned integration contract before
// any prompt or model work. Knowledge remains responsible for constructing
// valid versions; this boundary verifies that the copied objects are complete,
// mutually consistent, and closed over the fixed Corpus TextUnit set. The
// returned lookups are reused while building every Community context.
func validateReportInput(
	input Input,
) (map[string]Entity, map[string]Relation, error) {
	if strings.TrimSpace(input.Period) == "" {
		return nil, nil, fmt.Errorf(
			"%w: Report input Period is required",
			ErrInvalidReport,
		)
	}
	availableTextUnits, err := validateAvailableTextUnits(input.TextUnitIDs)
	if err != nil {
		return nil, nil, err
	}
	entities, err := validateReportEntities(input.Entities, availableTextUnits)
	if err != nil {
		return nil, nil, err
	}
	relations, err := validateRelations(
		input.Relations,
		entities,
		availableTextUnits,
	)
	if err != nil {
		return nil, nil, err
	}
	if err := validateReportClaims(
		input.Claims,
		entities,
		relations,
		availableTextUnits,
	); err != nil {
		return nil, nil, err
	}
	if err := validateReportCommunities(input.Communities, entities); err != nil {
		return nil, nil, err
	}
	return entities, relations, nil
}

func validateReportCommunities(
	values []community.Membership,
	entities map[string]Entity,
) error {
	if len(values) == 0 {
		return fmt.Errorf("%w: Report input requires Communities", ErrInvalidReport)
	}
	byID := make(map[community.CommunityID]community.Membership, len(values))
	seenNumbers := make(map[int]struct{}, len(values))
	children := make(map[community.CommunityID]int, len(values))
	rootEntities := make(map[string]struct{})
	finalEntities := make(map[string]int)
	for index, current := range values {
		if err := community.ValidateCommunityID(current.ID); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidReport, err)
		}
		if current.Number < 0 || current.Level < 0 {
			return fmt.Errorf("%w: Community %q has a negative Number or Level", ErrInvalidReport, current.ID)
		}
		if index > 0 {
			previous := values[index-1]
			if previous.Level > current.Level ||
				previous.Level == current.Level && previous.Number >= current.Number {
				return fmt.Errorf("%w: Communities must be ordered by Level and Number", ErrInvalidReport)
			}
		}
		if _, duplicate := byID[current.ID]; duplicate {
			return fmt.Errorf("%w: duplicate Community %q", ErrInvalidReport, current.ID)
		}
		if _, duplicate := seenNumbers[current.Number]; duplicate {
			return fmt.Errorf("%w: duplicate Community Number %d", ErrInvalidReport, current.Number)
		}
		seenNumbers[current.Number] = struct{}{}
		if len(current.EntityIDs) == 0 {
			return fmt.Errorf("%w: Community %q has no Entities", ErrInvalidReport, current.ID)
		}
		for memberIndex, entityID := range current.EntityIDs {
			if _, found := entities[entityID]; !found {
				return fmt.Errorf(
					"%w: Community %q Entity %q is absent from Report input",
					ErrInvalidReport,
					current.ID,
					entityID,
				)
			}
			if memberIndex > 0 && current.EntityIDs[memberIndex-1] >= entityID {
				return fmt.Errorf("%w: Community %q Entity IDs must be strictly ordered", ErrInvalidReport, current.ID)
			}
			if current.Level == 0 {
				rootEntities[entityID] = struct{}{}
			}
			if current.Final {
				finalEntities[entityID]++
			}
		}
		byID[current.ID] = current
	}
	for expected := range values {
		if _, found := seenNumbers[expected]; !found {
			return fmt.Errorf("%w: Community Numbers must be contiguous from zero", ErrInvalidReport)
		}
	}
	for _, current := range values {
		if current.ParentID == nil {
			if current.Level != 0 {
				return fmt.Errorf("%w: Community %q has no parent above Level zero", ErrInvalidReport, current.ID)
			}
			continue
		}
		parent, found := byID[*current.ParentID]
		if !found || parent.Level >= current.Level {
			return fmt.Errorf("%w: Community %q has an invalid parent", ErrInvalidReport, current.ID)
		}
		if !reportMembersContained(current.EntityIDs, parent.EntityIDs) {
			return fmt.Errorf("%w: Community %q has Entities outside its parent", ErrInvalidReport, current.ID)
		}
		children[parent.ID]++
	}
	for _, current := range values {
		if current.Final && children[current.ID] > 0 {
			return fmt.Errorf("%w: final Community %q has children", ErrInvalidReport, current.ID)
		}
		if !current.Final && children[current.ID] == 0 {
			return fmt.Errorf("%w: non-final Community %q has no children", ErrInvalidReport, current.ID)
		}
	}
	for entityID := range rootEntities {
		if finalEntities[entityID] != 1 {
			return fmt.Errorf("%w: Entity %q has %d final Communities", ErrInvalidReport, entityID, finalEntities[entityID])
		}
	}
	return nil
}

func reportMembersContained(values, parent []string) bool {
	allowed := make(map[string]struct{}, len(parent))
	for _, value := range parent {
		allowed[value] = struct{}{}
	}
	for _, value := range values {
		if _, found := allowed[value]; !found {
			return false
		}
	}
	return true
}

func validateReportEntities(
	values []Entity,
	availableTextUnits map[string]struct{},
) (map[string]Entity, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: Report input requires at least one Entity", ErrInvalidReport)
	}
	result := make(map[string]Entity, len(values))
	for index, entity := range values {
		if !uuid.IsCanonicalV4(entity.ID) {
			return nil, fmt.Errorf(
				"%w: Report Entity ID %q is not a canonical lowercase UUID v4",
				ErrInvalidReport,
				entity.ID,
			)
		}
		if err := validateReportVersion(entity.Version, "Entity", entity.ID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(entity.Title) == "" {
			return nil, fmt.Errorf(
				"%w: Report Entity %q Title is required",
				ErrInvalidReport,
				entity.ID,
			)
		}
		if entity.Degree < 0 {
			return nil, fmt.Errorf(
				"%w: Report Entity %q Degree must be non-negative",
				ErrInvalidReport,
				entity.ID,
			)
		}
		if err := validateReportEvidence(
			entity.TextUnitIDs,
			availableTextUnits,
			"Entity",
			entity.ID,
		); err != nil {
			return nil, err
		}
		if index > 0 && values[index-1].ID >= entity.ID {
			return nil, fmt.Errorf(
				"%w: Report Entities must be strictly ordered by ID",
				ErrInvalidReport,
			)
		}
		result[entity.ID] = entity
	}
	return result, nil
}

func validateRelations(
	values []Relation,
	entities map[string]Entity,
	availableTextUnits map[string]struct{},
) (map[string]Relation, error) {
	result := make(map[string]Relation, len(values))
	for index, relation := range values {
		if !uuid.IsCanonicalV4(relation.ID) {
			return nil, fmt.Errorf(
				"%w: Report Relation ID %q is not a canonical lowercase UUID v4",
				ErrInvalidReport,
				relation.ID,
			)
		}
		if err := validateReportVersion(
			relation.Version,
			"Relation",
			relation.ID,
		); err != nil {
			return nil, err
		}
		if _, found := entities[relation.SourceEntityID]; !found {
			return nil, fmt.Errorf(
				"%w: Report Relation %q Source Entity %q is absent",
				ErrInvalidReport,
				relation.ID,
				relation.SourceEntityID,
			)
		}
		if _, found := entities[relation.TargetEntityID]; !found {
			return nil, fmt.Errorf(
				"%w: Report Relation %q Target Entity %q is absent",
				ErrInvalidReport,
				relation.ID,
				relation.TargetEntityID,
			)
		}
		if math.IsNaN(relation.Weight) || math.IsInf(relation.Weight, 0) {
			return nil, fmt.Errorf(
				"%w: Report Relation %q Weight must be finite",
				ErrInvalidReport,
				relation.ID,
			)
		}
		if relation.CombinedDegree < 0 {
			return nil, fmt.Errorf(
				"%w: Report Relation %q CombinedDegree must be non-negative",
				ErrInvalidReport,
				relation.ID,
			)
		}
		if err := validateReportEvidence(
			relation.TextUnitIDs,
			availableTextUnits,
			"Relation",
			relation.ID,
		); err != nil {
			return nil, err
		}
		if index > 0 && values[index-1].ID >= relation.ID {
			return nil, fmt.Errorf(
				"%w: Report Relations must be strictly ordered by ID",
				ErrInvalidReport,
			)
		}
		result[relation.ID] = relation
	}
	return result, nil
}

func validateReportClaims(
	values []Claim,
	entities map[string]Entity,
	relations map[string]Relation,
	availableTextUnits map[string]struct{},
) error {
	for index, claim := range values {
		if !uuid.IsCanonicalV4(claim.ID) {
			return fmt.Errorf(
				"%w: Report Claim ID %q is not a canonical lowercase UUID v4",
				ErrInvalidReport,
				claim.ID,
			)
		}
		if err := validateReportVersion(claim.Version, "Claim", claim.ID); err != nil {
			return err
		}
		if claim.EvidenceIndex < 0 {
			return fmt.Errorf(
				"%w: Report Claim %q EvidenceIndex must be non-negative",
				ErrInvalidReport,
				claim.ID,
			)
		}
		if strings.TrimSpace(claim.SubjectText) == "" ||
			strings.TrimSpace(claim.Type) == "" ||
			strings.TrimSpace(claim.SourceText) == "" {
			return fmt.Errorf(
				"%w: Report Claim %q requires SubjectText, Type, and SourceText",
				ErrInvalidReport,
				claim.ID,
			)
		}
		if _, found := availableTextUnits[claim.TextUnitID]; !found {
			return fmt.Errorf(
				"%w: Report Claim %q references unavailable TextUnit %q",
				ErrInvalidReport,
				claim.ID,
				claim.TextUnitID,
			)
		}
		evidence, err := reportClaimSubjectEvidence(claim, entities, relations)
		if err != nil {
			return err
		}
		position := sort.SearchStrings(evidence, claim.TextUnitID)
		if position == len(evidence) || evidence[position] != claim.TextUnitID {
			return fmt.Errorf(
				"%w: Report Claim %q TextUnit %q is not evidence of Subject %q",
				ErrInvalidReport,
				claim.ID,
				claim.TextUnitID,
				claim.Subject.ID,
			)
		}

		if index == 0 || values[index-1].ID != claim.ID {
			if claim.EvidenceIndex != 0 {
				return fmt.Errorf(
					"%w: Report Claim %q Statements must start at index zero",
					ErrInvalidReport,
					claim.ID,
				)
			}
			if index > 0 && values[index-1].ID >= claim.ID {
				return fmt.Errorf(
					"%w: Report Claims must be ordered by ID and EvidenceIndex",
					ErrInvalidReport,
				)
			}
			continue
		}
		previous := values[index-1]
		if claim.EvidenceIndex != previous.EvidenceIndex+1 ||
			claim.Version != previous.Version ||
			claim.Type != previous.Type ||
			claim.Subject != previous.Subject {
			return fmt.Errorf(
				"%w: Report Claim %q Statements do not form one complete ordered Version",
				ErrInvalidReport,
				claim.ID,
			)
		}
	}
	return nil
}

func reportClaimSubjectEvidence(
	claim Claim,
	entities map[string]Entity,
	relations map[string]Relation,
) ([]string, error) {
	switch claim.Subject.Kind {
	case EntityClaimSubject:
		entity, found := entities[claim.Subject.ID]
		if !found {
			return nil, fmt.Errorf(
				"%w: Report Claim %q Subject Entity %q is absent",
				ErrInvalidReport,
				claim.ID,
				claim.Subject.ID,
			)
		}
		return entity.TextUnitIDs, nil
	case RelationClaimSubject:
		relation, found := relations[claim.Subject.ID]
		if !found {
			return nil, fmt.Errorf(
				"%w: Report Claim %q Subject Relation %q is absent",
				ErrInvalidReport,
				claim.ID,
				claim.Subject.ID,
			)
		}
		return relation.TextUnitIDs, nil
	default:
		return nil, fmt.Errorf(
			"%w: Report Claim %q Subject Kind %q is invalid",
			ErrInvalidReport,
			claim.ID,
			claim.Subject.Kind,
		)
	}
}

func validateReportEvidence(
	ids []string,
	availableTextUnits map[string]struct{},
	object string,
	id string,
) error {
	if len(ids) == 0 {
		return fmt.Errorf(
			"%w: Report %s %q requires TextUnit evidence",
			ErrInvalidReport,
			object,
			id,
		)
	}
	for index, textUnitID := range ids {
		if _, found := availableTextUnits[textUnitID]; !found {
			return fmt.Errorf(
				"%w: Report %s %q references unavailable TextUnit %q",
				ErrInvalidReport,
				object,
				id,
				textUnitID,
			)
		}
		if index > 0 && ids[index-1] >= textUnitID {
			return fmt.Errorf(
				"%w: Report %s %q TextUnit IDs must be strictly ordered",
				ErrInvalidReport,
				object,
				id,
			)
		}
	}
	return nil
}
