package readiness

import (
	"context"
	"strings"
	"testing"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/epoch"
	"github.com/memoria-space/meking/knowledge"
)

const (
	testCorporaID   = epoch.CorporaID("11111111-1111-4111-8111-111111111111")
	testStructureID = epoch.StructureID("22222222-2222-4222-8222-222222222222")
	testSetID       = community.CommunitySetID("33333333-3333-4333-8333-333333333333")
)

func TestPublicationReadinessValidatesExactStructureBoundary(t *testing.T) {
	fixture := newReadinessFixture()
	readiness := fixture.open(t)
	if err := readiness.Check(t.Context(), testStructureID, testCorporaID, fixture.versions); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !equalEntityReferences(fixture.entityVectors.entities, fixture.versions.Entities) {
		t.Fatalf("Entity vector references = %#v, want %#v", fixture.entityVectors.entities, fixture.versions.Entities)
	}
	if fixture.textVectors.corporaID != string(testCorporaID) || len(fixture.textVectors.units) != 3 {
		t.Fatalf("TextUnit vector request = %q/%d", fixture.textVectors.corporaID, len(fixture.textVectors.units))
	}
}

func TestPublicationReadinessRejectsStructureBoundaryMismatch(t *testing.T) {
	fixture := newReadinessFixture()
	fixture.structure.value.Knowledge.Entities[0].Version = 6
	err := fixture.open(t).Check(t.Context(), testStructureID, testCorporaID, fixture.versions)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestPublicationReadinessRequiresAllFacts(t *testing.T) {
	fixture := newReadinessFixture()
	if _, err := NewPublicationReadiness(
		fixture.corpora,
		nil,
		fixture.sets,
		fixture.entityVectors,
		fixture.textVectors,
	); err == nil {
		t.Fatal("NewPublicationReadiness() succeeded without Structures")
	}
}

type readinessFixture struct {
	versions      knowledge.Manifest
	corpora       *readinessCorpora
	structure     *readinessStructure
	sets          *readinessCommunitySets
	entityVectors *readinessEntityVectors
	textVectors   *readinessTextVectors
}

func newReadinessFixture() *readinessFixture {
	versions := knowledge.Manifest{
		Entities: []knowledge.Reference[knowledge.EntityID]{
			{ID: "44444444-4444-4444-8444-444444444444", Version: 7},
		},
	}
	return &readinessFixture{
		versions: versions,
		corpora: &readinessCorpora{value: corpus.Corpora{
			ID: corpus.CorporaID(testCorporaID),
			Texts: []corpus.ChunkedText{{TextUnits: []textunits.TextUnit{
				{TextUnit: textunits.TextUnitBody{ID: "tu-a"}},
				{TextUnit: textunits.TextUnitBody{ID: "tu-a"}},
				{TextUnit: textunits.TextUnitBody{ID: "tu-b"}},
			}}},
		}},
		structure: &readinessStructure{value: community.Structure{
			ID:             community.StructureID(testStructureID),
			CommunitySetID: testSetID,
			CorporaID:      string(testCorporaID),
			Knowledge:      versions.Clone(),
		}},
		sets:          &readinessCommunitySets{value: community.CommunitySet{ID: testSetID}},
		entityVectors: &readinessEntityVectors{},
		textVectors:   &readinessTextVectors{},
	}
}

func (fixture *readinessFixture) open(t *testing.T) *PublicationReadiness {
	t.Helper()
	readiness, err := NewPublicationReadiness(
		fixture.corpora,
		fixture.structure,
		fixture.sets,
		fixture.entityVectors,
		fixture.textVectors,
	)
	if err != nil {
		t.Fatalf("NewPublicationReadiness() error = %v", err)
	}
	return readiness
}

type readinessCorpora struct {
	value corpus.Corpora
}

func (reader *readinessCorpora) Corpora(context.Context, corpus.CorporaID) (corpus.Corpora, error) {
	return reader.value, nil
}

type readinessStructure struct {
	value community.Structure
}

func (reader *readinessStructure) LoadStructure(context.Context, community.StructureID) (community.Structure, error) {
	return reader.value, nil
}

type readinessCommunitySets struct {
	value community.CommunitySet
}

func (reader *readinessCommunitySets) Load(context.Context, community.CommunitySetID) (community.CommunitySet, error) {
	return reader.value, nil
}

type readinessEntityVectors struct {
	entities []knowledge.Reference[knowledge.EntityID]
}

func (reader *readinessEntityVectors) Validate(
	_ context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
) error {
	reader.entities = append([]knowledge.Reference[knowledge.EntityID](nil), entities...)
	return nil
}

func equalEntityReferences(
	left []knowledge.Reference[knowledge.EntityID],
	right []knowledge.Reference[knowledge.EntityID],
) bool {
	return knowledge.Manifest{Entities: left}.Equal(knowledge.Manifest{Entities: right})
}

type readinessTextVectors struct {
	corporaID string
	units     []textunits.TextUnitBody
}

func (reader *readinessTextVectors) Validate(
	_ context.Context,
	corporaID string,
	units []textunits.TextUnitBody,
) error {
	reader.corporaID = corporaID
	reader.units = append([]textunits.TextUnitBody(nil), units...)
	return nil
}

var _ epoch.Readiness = (*PublicationReadiness)(nil)
