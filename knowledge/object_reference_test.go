package knowledge_test

import (
	"testing"

	"github.com/memoria-space/meking/knowledge"
)

func TestObjectRefKeepsKindAndTypedIDTogether(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reference knowledge.ObjectRef
		id        string
	}{
		{knowledge.EntityID("entity-id"), "entity-id"},
		{knowledge.RelationID("relation-id"), "relation-id"},
		{knowledge.ClaimID("claim-id"), "claim-id"},
	}
	for _, test := range tests {
		if test.reference.ObjectID() != test.id {
			t.Fatalf("ObjectRef ID = %q, want %q", test.reference.ObjectID(), test.id)
		}
	}
}
