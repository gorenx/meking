package uuid_test

import (
	"testing"

	"github.com/memoria-space/meking/internal/uuid"
)

func TestNewV4ReturnsCanonicalDistinctValues(t *testing.T) {
	t.Parallel()

	first, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("NewV4() first: %v", err)
	}
	second, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("NewV4() second: %v", err)
	}
	if !uuid.IsCanonicalV4(first) {
		t.Fatalf("NewV4() = %q, want canonical lowercase UUID v4", first)
	}
	if !uuid.IsCanonicalV4(second) {
		t.Fatalf("NewV4() = %q, want canonical lowercase UUID v4", second)
	}
	if first == second {
		t.Fatalf("NewV4() returned duplicate value %q", first)
	}
}

func TestIsCanonicalV4RejectsOtherRepresentations(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"",
		"10000000-0000-3000-8000-000000000001",
		"10000000-0000-4000-7000-000000000001",
		"10000000-0000-4000-C000-000000000001",
		"10000000000040008000000000000001",
	} {
		if uuid.IsCanonicalV4(value) {
			t.Errorf("IsCanonicalV4(%q) = true, want false", value)
		}
	}
}
