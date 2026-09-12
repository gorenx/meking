package epoch

import (
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/knowledge"
)

const (
	testCorporaID   = CorporaID("1")
	testStructureID = StructureID("22222222-2222-4222-8222-222222222222")
)

func TestValidateEpochRequiresCompleteCanonicalValue(t *testing.T) {
	valid := Epoch{
		ID:          1,
		Knowledge:   testKnowledgeVersions(1),
		CorporaID:   testCorporaID,
		StructureID: testStructureID,
		PublishedAt: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		name   string
		mutate func(*Epoch)
	}{
		{
			name:   "ID",
			mutate: func(value *Epoch) { value.ID = 0 },
		},
		{
			name: "Knowledge",
			mutate: func(value *Epoch) {
				value.Knowledge.Entities[0].Version = 0
			},
		},
		{
			name:   "Corpora",
			mutate: func(value *Epoch) { value.CorporaID = "" },
		},
		{
			name:   "Structure",
			mutate: func(value *Epoch) { value.StructureID = "invalid" },
		},
		{
			name:   "PublishedAt",
			mutate: func(value *Epoch) { value.PublishedAt = time.Time{} },
		},
	}
	if err := ValidateEpoch(valid); err != nil {
		t.Fatalf("ValidateEpoch(valid): %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			test.mutate(&value)
			if err := ValidateEpoch(value); !errors.Is(err, ErrInvalidEpoch) {
				t.Fatalf("ValidateEpoch() error = %v", err)
			}
		})
	}
}

func TestValidatePublicationTargetRejectsNegativeIdentities(t *testing.T) {
	if err := ValidatePublicationTarget(PublicationTarget{StructureID: testStructureID}); err != nil {
		t.Fatalf("ValidatePublicationTarget(valid): %v", err)
	}
	for _, target := range []PublicationTarget{
		{
			ExpectedEpoch: -1,
			StructureID:   testStructureID,
		},
		{
			Knowledge: knowledge.Manifest{
				Entities: []knowledge.Reference[knowledge.EntityID]{{ID: "44444444-4444-4444-8444-444444444444"}},
			},
			StructureID: testStructureID,
		},
	} {
		if err := ValidatePublicationTarget(target); !errors.Is(err, ErrInvalidEpoch) {
			t.Fatalf("ValidatePublicationTarget(%+v) error = %v", target, err)
		}
	}
}

func testKnowledgeVersions(version knowledge.Version) knowledge.Manifest {
	return knowledge.Manifest{
		Entities: []knowledge.Reference[knowledge.EntityID]{
			{ID: "44444444-4444-4444-8444-444444444444", Version: version},
		},
	}
}
