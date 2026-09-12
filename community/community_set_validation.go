package community

import (
	"fmt"
	"math"
	"time"

	"github.com/memoria-space/meking/internal/uuid"
)

// ValidateCommunitySet verifies immutable identity, exact knowledge references,
// canonical order, hierarchy closure, membership containment, and timestamp
// semantics without changing the supplied value.
func ValidateCommunitySet(set CommunitySet) error {
	if err := ValidateCommunitySetID(set.ID); err != nil {
		return err
	}
	if set.DetectorVersion == 0 {
		return fmt.Errorf(
			"%w: CommunitySet DetectorVersion must be positive",
			ErrInvalidCommunitySet,
		)
	}
	if err := set.DetectionConfig.Validate(); err != nil {
		return fmt.Errorf(
			"%w: CommunitySet DetectionConfig is invalid: %v",
			ErrInvalidCommunitySet,
			err,
		)
	}
	if set.CreatedAt.IsZero() || set.CreatedAt.Location() != time.UTC {
		return fmt.Errorf(
			"%w: CommunitySet CreatedAt must be a non-zero UTC time",
			ErrInvalidCommunitySet,
		)
	}
	entityIDs, err := validateEntityReferences(set.Entities)
	if err != nil {
		return err
	}
	if err := validateRelationReferences(set.Relations); err != nil {
		return err
	}
	if len(set.Communities) == 0 {
		return fmt.Errorf(
			"%w: CommunitySet requires at least one Community",
			ErrInvalidCommunitySet,
		)
	}

	byID := make(map[CommunityID]Membership, len(set.Communities))
	numberByID := make(map[CommunityID]int, len(set.Communities))
	seenNumbers := make(map[int]struct{}, len(set.Communities))
	for index, current := range set.Communities {
		if index > 0 {
			previous := set.Communities[index-1]
			if previous.Level > current.Level ||
				previous.Level == current.Level && previous.Number >= current.Number {
				return fmt.Errorf(
					"%w: Communities must be strictly ordered by Level and Number",
					ErrInvalidCommunitySet,
				)
			}
		}
		if current.Number < 0 {
			return fmt.Errorf(
				"%w: Community Number must be non-negative",
				ErrInvalidCommunitySet,
			)
		}
		if _, duplicate := seenNumbers[current.Number]; duplicate {
			return fmt.Errorf(
				"%w: duplicate Community Number %d",
				ErrInvalidCommunitySet,
				current.Number,
			)
		}
		seenNumbers[current.Number] = struct{}{}
		if current.Level < 0 {
			return fmt.Errorf(
				"%w: Community %d Level must be non-negative",
				ErrInvalidCommunitySet,
				current.Number,
			)
		}
		if err := validateCommunityMembers(current, entityIDs); err != nil {
			return err
		}
		if _, duplicate := byID[current.ID]; duplicate {
			return fmt.Errorf(
				"%w: Community ID %q occurs more than once",
				ErrInvalidCommunitySet,
				current.ID,
			)
		}
		byID[current.ID] = current
		numberByID[current.ID] = current.Number
	}
	for expected := range set.Communities {
		if _, found := seenNumbers[expected]; !found {
			return fmt.Errorf(
				"%w: Community Numbers must be contiguous from zero",
				ErrInvalidCommunitySet,
			)
		}
	}

	detected := Hierarchy{Communities: make([]Community, len(set.Communities))}
	for index, current := range set.Communities {
		parentNumber := -1
		if current.ParentID == nil {
			if current.Level != 0 {
				return fmt.Errorf(
					"%w: Community %d has no parent above Level zero",
					ErrInvalidCommunitySet,
					current.Number,
				)
			}
		} else {
			parent, found := byID[*current.ParentID]
			if !found {
				return fmt.Errorf(
					"%w: Community %d references missing parent %q",
					ErrInvalidCommunitySet,
					current.Number,
					*current.ParentID,
				)
			}
			if parent.Level >= current.Level {
				return fmt.Errorf(
					"%w: Community %d parent is not from an earlier Level",
					ErrInvalidCommunitySet,
					current.Number,
				)
			}
			parentNumber = numberByID[*current.ParentID]
		}
		detected.Communities[index] = Community{
			ID:           current.Number,
			Level:        current.Level,
			ParentID:     parentNumber,
			Nodes:        append([]string(nil), current.EntityIDs...),
			Final:        current.Final,
			Unsplittable: current.Unsplittable,
		}
	}
	if err := detected.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCommunitySet, err)
	}
	return nil
}

func validateEntityReferences(values []EntityReference) (map[string]struct{}, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf(
			"%w: CommunitySet requires at least one Entity reference",
			ErrInvalidCommunitySet,
		)
	}
	result := make(map[string]struct{}, len(values))
	for index, reference := range values {
		if !uuid.IsCanonicalV4(reference.ID) {
			return nil, fmt.Errorf(
				"%w: Entity ID %q is not a canonical lowercase UUID v4",
				ErrInvalidCommunitySet,
				reference.ID,
			)
		}
		if err := validateKnowledgeVersion(reference.Version, "Entity", reference.ID); err != nil {
			return nil, err
		}
		if index > 0 && values[index-1].ID >= reference.ID {
			return nil, fmt.Errorf(
				"%w: Entity references must be strictly ordered by ID",
				ErrInvalidCommunitySet,
			)
		}
		result[reference.ID] = struct{}{}
	}
	return result, nil
}

func validateRelationReferences(values []RelationReference) error {
	if len(values) == 0 {
		return fmt.Errorf(
			"%w: CommunitySet requires at least one Relation reference",
			ErrInvalidCommunitySet,
		)
	}
	for index, reference := range values {
		if !uuid.IsCanonicalV4(reference.ID) {
			return fmt.Errorf(
				"%w: Relation ID %q is not a canonical lowercase UUID v4",
				ErrInvalidCommunitySet,
				reference.ID,
			)
		}
		if err := validateKnowledgeVersion(reference.Version, "Relation", reference.ID); err != nil {
			return err
		}
		if index > 0 && values[index-1].ID >= reference.ID {
			return fmt.Errorf(
				"%w: Relation references must be strictly ordered by ID",
				ErrInvalidCommunitySet,
			)
		}
	}
	return nil
}

func validateKnowledgeVersion(version uint64, object string, id string) error {
	if version == 0 || version > math.MaxInt64 {
		return fmt.Errorf(
			"%w: %s %q Version must fit a positive SQLite INTEGER",
			ErrInvalidCommunitySet,
			object,
			id,
		)
	}
	return nil
}

func validateCommunityMembers(
	current Membership,
	entityIDs map[string]struct{},
) error {
	if err := ValidateCommunityID(current.ID); err != nil {
		return err
	}
	if len(current.EntityIDs) == 0 {
		return fmt.Errorf(
			"%w: Community %d requires at least one Entity",
			ErrInvalidCommunitySet,
			current.Number,
		)
	}
	for index, entityID := range current.EntityIDs {
		if !uuid.IsCanonicalV4(entityID) {
			return fmt.Errorf(
				"%w: Community %d Entity ID %q is not a canonical lowercase UUID v4",
				ErrInvalidCommunitySet,
				current.Number,
				entityID,
			)
		}
		if index > 0 && current.EntityIDs[index-1] >= entityID {
			return fmt.Errorf(
				"%w: Community %d Entity IDs must be strictly ordered",
				ErrInvalidCommunitySet,
				current.Number,
			)
		}
		if _, found := entityIDs[entityID]; !found {
			return fmt.Errorf(
				"%w: Community %d Entity %q has no version reference",
				ErrInvalidCommunitySet,
				current.Number,
				entityID,
			)
		}
	}
	if expected := communityID(current.EntityIDs); current.ID != expected {
		return fmt.Errorf(
			"%w: Community %d ID does not match its Entity membership",
			ErrInvalidCommunitySet,
			current.Number,
		)
	}
	return nil
}
