package extraction

import (
	"reflect"
	"testing"
)

func TestDefaultStandardKnowledgeConfigurationsAreIndependent(t *testing.T) {
	firstExtraction := DefaultGraphExtractionConfig()
	secondExtraction := DefaultGraphExtractionConfig()
	wantEntityTypes := []string{"organization", "person", "geo", "event"}
	if !reflect.DeepEqual(firstExtraction.EntityTypes, wantEntityTypes) ||
		firstExtraction.MaxGleanings != DefaultGraphExtractionMaxGleanings {
		t.Fatalf("graph extraction defaults = %#v", firstExtraction)
	}
	firstExtraction.EntityTypes[0] = "changed"
	if reflect.DeepEqual(firstExtraction.EntityTypes, secondExtraction.EntityTypes) {
		t.Fatal("graph extraction defaults share their entity type slice")
	}

	claims := DefaultClaimExtractionConfig()
	if claims.Description != DefaultClaimDescription || claims.MaxGleanings != DefaultClaimMaxGleanings {
		t.Fatalf("Claim defaults = %#v", claims)
	}
}

func TestStandardKnowledgeConfigurationsOwnTheirBounds(t *testing.T) {
	if err := (GraphExtractionConfig{MaxGleanings: -1}).Validate(); err == nil {
		t.Fatal("negative graph extraction gleanings accepted")
	}
	if err := (ClaimExtractionConfig{MaxGleanings: -1}).Validate(); err == nil {
		t.Fatal("negative Claim extraction gleanings accepted")
	}
}
