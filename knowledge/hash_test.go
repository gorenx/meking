package knowledge_test

import (
	"testing"

	"github.com/memoria-space/meking/knowledge"
)

func TestEntityHashCoversOnlyCanonicalContent(t *testing.T) {
	base := testEntity()
	baseHash, err := knowledge.HashEntity(base)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*knowledge.Entity){
		"aliases":     func(value *knowledge.Entity) { value.Aliases = []string{"OTHER"} },
		"description": func(value *knowledge.Entity) { value.Description = "Other" },
	} {
		value := base
		change(&value)
		hash, err := knowledge.HashEntity(value)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if hash == baseHash {
			t.Fatalf("%s did not change Entity Hash", name)
		}
	}
	otherID := base
	otherID.ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	otherID.Title = "Other Identity"
	otherID.Type = "OTHER"
	hash, err := knowledge.HashEntity(otherID)
	if err != nil {
		t.Fatal(err)
	}
	if hash != baseHash {
		t.Fatal("stable Entity identity changed canonical content Hash")
	}
}

func TestRelationAndClaimHashOnlyDescription(t *testing.T) {
	relation := testRelation()
	relationHash, err := knowledge.HashRelation(relation)
	if err != nil {
		t.Fatal(err)
	}
	relation.ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	relation.SourceEntityID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	relation.TargetEntityID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	relation.Type = "OTHER"
	if hash, err := knowledge.HashRelation(relation); err != nil || hash != relationHash {
		t.Fatalf("Relation identity affected Hash: %v", err)
	}

	claim := testClaim(t)
	claimHash, err := knowledge.HashClaim(claim)
	if err != nil {
		t.Fatal(err)
	}
	claim.ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	claim.Subject = knowledge.EntityID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	claim.Type = "OTHER"
	if hash, err := knowledge.HashClaim(claim); err != nil || hash != claimHash {
		t.Fatalf("Claim identity affected Hash: %v", err)
	}
}

func testEntity() knowledge.Entity {
	return knowledge.Entity{
		ID: "11111111-1111-4111-8111-111111111111", Title: "Entity",
		Type: "ORGANIZATION", Aliases: []string{"EA", "ENTITY ALPHA"}, Description: "Description",
	}
}

func testRelation() knowledge.Relation {
	return knowledge.Relation{
		ID:             "33333333-3333-4333-8333-333333333333",
		SourceEntityID: "11111111-1111-4111-8111-111111111111",
		TargetEntityID: "22222222-2222-4222-8222-222222222222",
		Type:           "RELATED_TO", Description: "Relation",
	}
}

func testClaim(t *testing.T) knowledge.Claim {
	t.Helper()
	subject, err := knowledge.NewRelationSubject("33333333-3333-4333-8333-333333333333")
	if err != nil {
		t.Fatal(err)
	}
	return knowledge.Claim{
		ID: "44444444-4444-4444-8444-444444444444", Subject: subject,
		Type: "FACT", Description: "A is connected to B",
	}
}
