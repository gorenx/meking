package zone

import (
	"errors"
	"testing"
	"time"
)

const (
	rootID  ID = "10000000-0000-4000-8000-000000000001"
	childID ID = "10000000-0000-4000-8000-000000000002"
)

func TestIDGenerationAndParsing(t *testing.T) {
	t.Parallel()

	first, err := NewID()
	if err != nil {
		t.Fatalf("NewID() first error = %v", err)
	}
	second, err := NewID()
	if err != nil {
		t.Fatalf("NewID() second error = %v", err)
	}
	if first == second {
		t.Fatalf("NewID() returned duplicate ID %q", first)
	}
	if parsed, err := ParseID(string(first)); err != nil || parsed != first {
		t.Fatalf("ParseID(%q) = %q, %v", first, parsed, err)
	}
	if _, err := ParseID("invalid"); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("ParseID(invalid) error = %v", err)
	}
}

func TestDefinitionConstructorsEnforceHierarchy(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.August, 3, 1, 2, 3, 0, time.FixedZone("test", 3600))
	root, err := NewRootDefinition(rootID, "user-1", createdAt)
	if err != nil {
		t.Fatalf("NewRootDefinition() error = %v", err)
	}
	if root.Role != RoleRoot || root.UserID != "user-1" || root.CreatedAt.Location() != time.UTC {
		t.Fatalf("Root Definition = %#v", root)
	}
	if _, ok := root.Parent(); ok {
		t.Fatal("Root Parent() reported a Parent")
	}

	child, err := NewChildDefinition(childID, rootID, createdAt)
	if err != nil {
		t.Fatalf("NewChildDefinition() error = %v", err)
	}
	if parent, ok := child.Parent(); !ok || parent != rootID {
		t.Fatalf("Child Parent() = %q, %v", parent, ok)
	}
}

func TestDefinitionConstructorsRejectInvalidFacts(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().UTC()
	if _, err := NewRootDefinition("invalid", "user-1", createdAt); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("invalid Root ID error = %v", err)
	}
	if _, err := NewRootDefinition(rootID, "user-1", time.Time{}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("missing creation time error = %v", err)
	}
	if _, err := NewRootDefinition(rootID, " user-1", createdAt); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("invalid User ID error = %v", err)
	}
	if _, err := NewChildDefinition(childID, "invalid", createdAt); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("invalid Parent ID error = %v", err)
	}
	if _, err := NewChildDefinition(rootID, rootID, createdAt); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("self Parent error = %v", err)
	}
}
