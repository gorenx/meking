package knowledge

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/memoria-space/meking/internal/uuid"
)

const entityReferenceSeparator = "@"

// EntityIdentity is the canonical business identity permanently bound to one
// EntityID. Title and Type have surrounding Unicode whitespace removed before
// the identity enters a Knowledge transaction.
type EntityIdentity struct {
	Title string `json:"title"`
	Type  string `json:"type"`
}

// RelationKey is the resolved endpoint-and-Type key bound to one RelationID.
type RelationKey struct {
	SourceEntityID EntityID
	TargetEntityID EntityID
	Type           string
}

// ClaimIdentity is the permanent Subject-and-Type binding for one ClaimID.
type ClaimIdentity struct {
	Subject Subject
	Type    string
}

// NewEntityIdentity canonicalizes and validates the stable Entity identity.
func NewEntityIdentity(title string, entityType string) (EntityIdentity, error) {
	identity := EntityIdentity{
		Title: strings.TrimSpace(title),
		Type:  strings.TrimSpace(entityType),
	}
	if identity.Title == "" {
		return EntityIdentity{}, fmt.Errorf("%w: entity title is required", ErrInvalidChange)
	}
	if identity.Type == "" {
		return EntityIdentity{}, fmt.Errorf("%w: entity type is required", ErrInvalidChange)
	}
	return identity, nil
}

func NewRelationKey(source EntityID, target EntityID, relationType string) (RelationKey, error) {
	if err := ValidateEntityID(source); err != nil {
		return RelationKey{}, err
	}
	if err := ValidateEntityID(target); err != nil {
		return RelationKey{}, err
	}
	key := RelationKey{
		SourceEntityID: source,
		TargetEntityID: target,
		Type:           strings.TrimSpace(relationType),
	}
	if key.Type == "" {
		return RelationKey{}, fmt.Errorf("%w: relation type is required", ErrInvalidChange)
	}
	return key, nil
}

func NewClaimIdentity(subject Subject, claimType string) (ClaimIdentity, error) {
	if err := validateSubject(subject); err != nil {
		return ClaimIdentity{}, err
	}
	identity := ClaimIdentity{Subject: subject, Type: strings.TrimSpace(claimType)}
	if identity.Type == "" {
		return ClaimIdentity{}, fmt.Errorf("%w: claim type is required", ErrInvalidChange)
	}
	return identity, nil
}

// NewEntityID allocates a canonical lowercase UUID v4 for a newly resolved
// Entity identity. The caller persists it only after identity resolution decides
// that no existing Entity matches.
func NewEntityID() (EntityID, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("create entity ID: %w", err)
	}
	return EntityID(id), nil
}

// NewRelationID allocates a canonical lowercase UUID v4 for a directed endpoint
// and Type binding that has no existing Relation identity.
func NewRelationID() (RelationID, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("create relation ID: %w", err)
	}
	return RelationID(id), nil
}

// NewClaimID allocates a canonical lowercase UUID v4 after extraction finds no ID
// bound to the exact (Subject reference, Type) Claim identity. Allocation
// does not group extraction results or decide which content belongs together.
func NewClaimID() (ClaimID, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("create claim ID: %w", err)
	}
	return ClaimID(id), nil
}

// ValidateEntityID verifies the canonical lowercase UUID v4 format required for
// an Entity identity.
func ValidateEntityID(id EntityID) error {
	if !uuid.IsCanonicalV4(string(id)) {
		return fmt.Errorf("%w: entity ID %q is not a canonical lowercase UUID v4", ErrInvalidChange, id)
	}
	return nil
}

// ValidateRelationID verifies the canonical lowercase UUID v4 format required
// for a Relation identity.
func ValidateRelationID(id RelationID) error {
	if !uuid.IsCanonicalV4(string(id)) {
		return fmt.Errorf("%w: relation ID %q is not a canonical lowercase UUID v4", ErrInvalidChange, id)
	}
	return nil
}

// ValidateClaimID verifies the canonical lowercase UUID v4 format required for
// a Claim identity.
func ValidateClaimID(id ClaimID) error {
	if !uuid.IsCanonicalV4(string(id)) {
		return fmt.Errorf("%w: claim ID %q is not a canonical lowercase UUID v4", ErrInvalidChange, id)
	}
	return nil
}

// FormatEntityReference serializes one exact Entity ID and Version for an
// opaque cross-context Vector ID. The text remains a reference encoding and
// does not introduce an independently addressable Version ID.
func FormatEntityReference(reference Reference[EntityID]) (string, error) {
	if err := ValidateEntityID(reference.ID); err != nil {
		return "", err
	}
	if reference.Version == 0 {
		return "", fmt.Errorf("%w: entity reference Version must be positive", ErrInvalidChange)
	}
	return string(reference.ID) + entityReferenceSeparator +
		strconv.FormatUint(uint64(reference.Version), 10), nil
}

// ParseEntityReference restores the exact Entity reference encoded by
// FormatEntityReference and rejects alternate spellings such as leading-zero
// versions so one reference has one Vector ID representation.
func ParseEntityReference(value string) (Reference[EntityID], error) {
	separator := strings.LastIndex(value, entityReferenceSeparator)
	if separator <= 0 || separator == len(value)-1 {
		return Reference[EntityID]{}, fmt.Errorf(
			"%w: entity reference %q is invalid",
			ErrInvalidChange,
			value,
		)
	}
	id := EntityID(value[:separator])
	if err := ValidateEntityID(id); err != nil {
		return Reference[EntityID]{}, err
	}
	versionText := value[separator+len(entityReferenceSeparator):]
	version, err := strconv.ParseUint(versionText, 10, 64)
	if err != nil || version == 0 || strconv.FormatUint(version, 10) != versionText {
		return Reference[EntityID]{}, fmt.Errorf(
			"%w: entity reference Version %q is invalid",
			ErrInvalidChange,
			versionText,
		)
	}
	return Reference[EntityID]{ID: id, Version: Version(version)}, nil
}

func validateSubject(subject Subject) error {
	switch subject := subject.(type) {
	case EntityID:
		return ValidateEntityID(subject)
	case RelationID:
		return ValidateRelationID(subject)
	default:
		return fmt.Errorf("%w: unsupported claim Subject %T", ErrInvalidChange, subject)
	}
}
