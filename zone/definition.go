package zone

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/memoria-space/meking/internal/uuid"
)

// ID is the stable identity of one Zone.
type ID string

// NewID creates a new Zone identity.
func NewID() (ID, error) {
	value, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("create Zone ID: %w", err)
	}
	return ID(value), nil
}

// ParseID validates and restores one Zone identity.
func ParseID(value string) (ID, error) {
	if !uuid.IsCanonicalV4(value) {
		return "", fmt.Errorf(
			"%w: Zone ID %q is not a canonical lowercase UUID v4",
			ErrInvalidDefinition,
			value,
		)
	}
	return ID(value), nil
}

// Role distinguishes a Root Zone from its direct Child.
type Role string

const (
	RoleRoot  Role = "root"
	RoleChild Role = "child"
)

// Definition is the stable Zone identity and hierarchy record.
type Definition struct {
	ID        ID
	Role      Role
	ParentID  *ID
	UserID    string
	CreatedAt time.Time
}

// NewRootDefinition creates a Root Zone definition.
func NewRootDefinition(id ID, userID string, createdAt time.Time) (Definition, error) {
	definition := Definition{
		ID:        id,
		Role:      RoleRoot,
		UserID:    userID,
		CreatedAt: createdAt.UTC(),
	}
	if err := validateDefinition(definition); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

// NewChildDefinition creates a direct Child Zone definition.
func NewChildDefinition(id ID, parentID ID, createdAt time.Time) (Definition, error) {
	parent := parentID
	definition := Definition{
		ID: id, Role: RoleChild, ParentID: &parent, CreatedAt: createdAt.UTC(),
	}
	if err := validateDefinition(definition); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

// Parent returns the direct Parent identity for a Child Zone.
func (definition Definition) Parent() (ID, bool) {
	if definition.Role != RoleChild || definition.ParentID == nil {
		return "", false
	}
	return *definition.ParentID, true
}

func validateDefinition(definition Definition) error {
	if _, err := ParseID(string(definition.ID)); err != nil {
		return err
	}
	if definition.CreatedAt.IsZero() {
		return fmt.Errorf("%w: creation time is required", ErrInvalidDefinition)
	}
	switch definition.Role {
	case RoleRoot:
		if definition.ParentID != nil {
			return fmt.Errorf("%w: Root Zone cannot have a Parent", ErrInvalidDefinition)
		}
		if definition.UserID != "" {
			if err := validateUserID(definition.UserID); err != nil {
				return err
			}
		}
	case RoleChild:
		if definition.ParentID == nil {
			return ErrParentRequired
		}
		if _, err := ParseID(string(*definition.ParentID)); err != nil {
			return fmt.Errorf("%w: invalid Parent: %v", ErrInvalidDefinition, err)
		}
		if definition.ID == *definition.ParentID {
			return fmt.Errorf("%w: Zone cannot be its own Parent", ErrInvalidDefinition)
		}
		if definition.UserID != "" {
			return fmt.Errorf("%w: Child Zone cannot identify a User", ErrInvalidDefinition)
		}
	default:
		return fmt.Errorf("%w: unsupported role %q", ErrInvalidDefinition, definition.Role)
	}
	return nil
}

func validateUserID(userID string) error {
	if userID == "" || userID != strings.TrimSpace(userID) || !utf8.ValidString(userID) ||
		len(userID) > 256 || strings.ContainsAny(userID, "/\x00") {
		return fmt.Errorf("%w: User ID must be a non-empty URI path segment of at most 256 bytes", ErrInvalidUser)
	}
	return nil
}
