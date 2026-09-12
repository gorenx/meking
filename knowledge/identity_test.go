package knowledge_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/memoria-space/meking/knowledge"
)

func TestGeneratedIDsAreCanonicalAndUnique(t *testing.T) {
	t.Parallel()

	entities := make(map[knowledge.EntityID]struct{})
	relations := make(map[knowledge.RelationID]struct{})
	claims := make(map[knowledge.ClaimID]struct{})
	for range 100 {
		entityID, err := knowledge.NewEntityID()
		if err != nil {
			t.Fatalf("new Entity ID: %v", err)
		}
		if err := knowledge.ValidateEntityID(entityID); err != nil {
			t.Fatalf("validate generated Entity ID %q: %v", entityID, err)
		}
		if strings.ToLower(string(entityID)) != string(entityID) {
			t.Fatalf("Entity ID is not lowercase: %q", entityID)
		}
		if _, duplicate := entities[entityID]; duplicate {
			t.Fatalf("duplicate generated Entity ID %q", entityID)
		}
		entities[entityID] = struct{}{}

		relationID, err := knowledge.NewRelationID()
		if err != nil {
			t.Fatalf("new Relation ID: %v", err)
		}
		if err := knowledge.ValidateRelationID(relationID); err != nil {
			t.Fatalf("validate generated Relation ID %q: %v", relationID, err)
		}
		if _, duplicate := relations[relationID]; duplicate {
			t.Fatalf("duplicate generated Relation ID %q", relationID)
		}
		relations[relationID] = struct{}{}

		claimID, err := knowledge.NewClaimID()
		if err != nil {
			t.Fatalf("new Claim ID: %v", err)
		}
		if err := knowledge.ValidateClaimID(claimID); err != nil {
			t.Fatalf("validate generated Claim ID %q: %v", claimID, err)
		}
		if _, duplicate := claims[claimID]; duplicate {
			t.Fatalf("duplicate generated Claim ID %q", claimID)
		}
		claims[claimID] = struct{}{}
	}
}

func TestEntityReferenceTextRoundTrip(t *testing.T) {
	t.Parallel()

	reference := knowledge.Reference[knowledge.EntityID]{
		ID:      "11111111-1111-4111-8111-111111111111",
		Version: 42,
	}
	encoded, err := knowledge.FormatEntityReference(reference)
	if err != nil {
		t.Fatalf("FormatEntityReference() error = %v", err)
	}
	if encoded != "11111111-1111-4111-8111-111111111111@42" {
		t.Fatalf("FormatEntityReference() = %q", encoded)
	}
	parsed, err := knowledge.ParseEntityReference(encoded)
	if err != nil {
		t.Fatalf("ParseEntityReference() error = %v", err)
	}
	if !reflect.DeepEqual(parsed, reference) {
		t.Fatalf("ParseEntityReference() = %#v, want %#v", parsed, reference)
	}

	for _, invalid := range []string{
		"",
		"11111111-1111-4111-8111-111111111111",
		"11111111-1111-4111-8111-111111111111@0",
		"11111111-1111-4111-8111-111111111111@01",
		"11111111-1111-4111-8111-111111111111@version",
	} {
		if _, err := knowledge.ParseEntityReference(invalid); err == nil {
			t.Errorf("ParseEntityReference(%q) error = nil", invalid)
		}
	}
}

func TestIDValidationRejectsNonCanonicalUUIDs(t *testing.T) {
	t.Parallel()

	invalid := []knowledge.EntityID{
		"",
		"11111111-1111-3111-8111-111111111111",
		"11111111-1111-4111-7111-111111111111",
		"11111111-1111-4111-8111-11111111111A",
		"11111111111141118111111111111111",
	}
	for _, id := range invalid {
		if err := knowledge.ValidateEntityID(id); err == nil {
			t.Errorf("ValidateEntityID(%q) succeeded", id)
		}
		if err := knowledge.ValidateClaimID(knowledge.ClaimID(id)); err == nil {
			t.Errorf("ValidateClaimID(%q) succeeded", id)
		}
	}
}

func TestRelationAndClaimIdentitiesUsePersistentReferences(t *testing.T) {
	t.Parallel()

	source := knowledge.EntityID("11111111-1111-4111-8111-111111111111")
	target := knowledge.EntityID("22222222-2222-4222-8222-222222222222")
	relation, err := knowledge.NewRelationKey(source, target, " ASSOCIATED_WITH ")
	if err != nil {
		t.Fatalf("NewRelationKey() error = %v", err)
	}
	if relation.SourceEntityID != source || relation.TargetEntityID != target || relation.Type != "ASSOCIATED_WITH" {
		t.Fatalf("RelationKey = %#v", relation)
	}
	subject, err := knowledge.NewEntitySubject(source)
	if err != nil {
		t.Fatalf("NewEntitySubject() error = %v", err)
	}
	claim, err := knowledge.NewClaimIdentity(subject, " FACT ")
	if err != nil {
		t.Fatalf("NewClaimIdentity() error = %v", err)
	}
	if claim.Subject != subject || claim.Type != "FACT" {
		t.Fatalf("ClaimIdentity = %#v", claim)
	}
	if _, err := knowledge.NewClaimIdentity(subject, " "); err == nil {
		t.Fatal("NewClaimIdentity() accepted an empty Type")
	}
}
