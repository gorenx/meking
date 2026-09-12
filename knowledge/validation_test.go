package knowledge_test

import (
	"errors"
	"testing"

	"github.com/memoria-space/meking/knowledge"
)

func TestValidateEntityRequiresCanonicalIdentityAndAliases(t *testing.T) {
	entity := testEntity()
	entity.Title = " Entity "
	if err := knowledge.ValidateEntity(entity); !errors.Is(err, knowledge.ErrInvalidChange) {
		t.Fatalf("unnormalized identity error = %v", err)
	}
	for _, aliases := range [][]string{{""}, {"Entity"}, {"ENTITY ALPHA", "EA"}, {"EA", "EA"}} {
		entity = testEntity()
		entity.Aliases = aliases
		if err := knowledge.ValidateEntity(entity); !errors.Is(err, knowledge.ErrInvalidChange) {
			t.Fatalf("aliases %#v error = %v", aliases, err)
		}
	}
}

func TestNextVersionRejectsZeroAndExhaustion(t *testing.T) {
	if _, err := knowledge.NextVersion(0); !errors.Is(err, knowledge.ErrVersionExhausted) {
		t.Fatalf("NextVersion(0) error = %v", err)
	}
	if got, err := knowledge.NextVersion(2); err != nil || got != 3 {
		t.Fatalf("NextVersion(2) = %d, %v", got, err)
	}
}
