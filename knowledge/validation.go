package knowledge

import (
	"fmt"
	"math"
	"strings"
)

const maximumVersion = Version(math.MaxInt64)

// ValidateEntity checks the stable identity and canonical content stored in an
// immutable Entity Version.
func ValidateEntity(entity Entity) error {
	if err := ValidateEntityID(entity.ID); err != nil {
		return err
	}
	return ValidateEntityContent(EntityContent{
		Identity: EntityIdentity{Title: entity.Title, Type: entity.Type},
		Aliases:  entity.Aliases, Description: entity.Description,
	})
}

// ValidateEntityContent checks an Entity value before its persistent ID is
// resolved.
func ValidateEntityContent(content EntityContent) error {
	identity, err := NewEntityIdentity(content.Identity.Title, content.Identity.Type)
	if err != nil {
		return err
	}
	if identity != content.Identity {
		return fmt.Errorf(
			"%w: entity Title and Type must use their canonical identity values",
			ErrInvalidChange,
		)
	}
	for index, alias := range content.Aliases {
		if strings.TrimSpace(alias) == "" {
			return fmt.Errorf("%w: entity aliases contain an empty value", ErrInvalidChange)
		}
		if alias == content.Identity.Title {
			return fmt.Errorf("%w: entity aliases must not contain Title", ErrInvalidChange)
		}
		if index > 0 && content.Aliases[index-1] >= alias {
			return fmt.Errorf(
				"%w: entity aliases must be strictly sorted and deduplicated",
				ErrInvalidChange,
			)
		}
	}
	return validateDescription("entity", content.Description)
}

// ValidateRelation checks the directed endpoint-and-Type identity and the
// canonical content stored in an immutable Relation Version.
func ValidateRelation(relation Relation) error {
	if err := ValidateRelationID(relation.ID); err != nil {
		return err
	}
	identity, err := NewRelationKey(
		relation.SourceEntityID,
		relation.TargetEntityID,
		relation.Type,
	)
	if err != nil {
		return err
	}
	if identity.Type != relation.Type {
		return fmt.Errorf("%w: relation Type must use its canonical identity value", ErrInvalidChange)
	}
	return validateDescription("relation", relation.Description)
}

// ValidateRelationContent checks a Relation value before its endpoint IDs are
// resolved.
func ValidateRelationContent(content RelationContent) error {
	if err := validateRelationIdentity(content.Source, content.Target, content.Type); err != nil {
		return err
	}
	return validateDescription("relation", content.Description)
}

func validateRelationIdentity(sourceIdentity EntityIdentity, targetIdentity EntityIdentity, relationType string) error {
	source, err := NewEntityIdentity(sourceIdentity.Title, sourceIdentity.Type)
	if err != nil {
		return err
	}
	if source != sourceIdentity {
		return fmt.Errorf("%w: relation Source must use its canonical identity values", ErrInvalidChange)
	}
	target, err := NewEntityIdentity(targetIdentity.Title, targetIdentity.Type)
	if err != nil {
		return err
	}
	if target != targetIdentity {
		return fmt.Errorf("%w: relation Target must use its canonical identity values", ErrInvalidChange)
	}
	if strings.TrimSpace(relationType) == "" {
		return fmt.Errorf("%w: relation Type is required", ErrInvalidChange)
	}
	if strings.TrimSpace(relationType) != relationType {
		return fmt.Errorf("%w: relation Type must use its canonical identity value", ErrInvalidChange)
	}
	return nil
}

// ValidateClaim checks the Subject-and-Type identity and the single Claim
// description stored in an immutable Claim Version.
func ValidateClaim(claim Claim) error {
	if err := ValidateClaimID(claim.ID); err != nil {
		return err
	}
	identity, err := NewClaimIdentity(claim.Subject, claim.Type)
	if err != nil {
		return err
	}
	if identity.Type != claim.Type {
		return fmt.Errorf("%w: claim Type must use its canonical identity value", ErrInvalidChange)
	}
	return validateDescription("claim", claim.Description)
}

// ValidateClaimContent checks a Claim value before its Subject is resolved to
// a persistent ID.
func ValidateClaimContent(content ClaimContent) error {
	switch subject := content.Subject.(type) {
	case EntityIdentity:
		identity, err := NewEntityIdentity(subject.Title, subject.Type)
		if err != nil {
			return err
		}
		if identity != subject {
			return fmt.Errorf("%w: claim Entity Subject must use its canonical identity values", ErrInvalidChange)
		}
	case RelationIdentity:
		if err := validateRelationIdentity(subject.Source, subject.Target, subject.Type); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: unsupported Claim Subject %T", ErrInvalidChange, content.Subject)
	}
	if strings.TrimSpace(content.Type) == "" {
		return fmt.Errorf("%w: claim Type is required", ErrInvalidChange)
	}
	if strings.TrimSpace(content.Type) != content.Type {
		return fmt.Errorf("%w: claim Type must use its canonical identity value", ErrInvalidChange)
	}
	return validateDescription("claim", content.Description)
}

func validateDescription(kind string, description string) error {
	if strings.TrimSpace(description) == "" {
		return fmt.Errorf("%w: %s Description is required", ErrInvalidChange, kind)
	}
	return nil
}

// ValidateVersion rejects values that cannot be represented by SQLite. Zero is
// accepted only by command fields whose contract explicitly defines a sentinel.
func ValidateVersion(version Version) error {
	if version > maximumVersion {
		return fmt.Errorf("%w: version %d exceeds SQLite INTEGER", ErrInvalidChange, version)
	}
	return nil
}

func NextVersion(version Version) (Version, error) {
	if version == 0 || version >= maximumVersion {
		return 0, ErrVersionExhausted
	}
	return version + 1, nil
}
