package memory

import (
	"context"
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

const searchEntityID = knowledge.EntityID("11111111-1111-4111-8111-111111111111")

func TestSearcherUsesExactEntityIdentityBeforeSemanticSearch(t *testing.T) {
	version := searchEntityVersion()
	identities := &identityIndex{
		entities: knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID]{
			Items: []knowledge.Reference[knowledge.EntityID]{
				{
					ID:      searchEntityID,
					Version: version.Version,
				},
			},
		},
	}
	entities := &versions[knowledge.Entity, knowledge.EntityID]{
		readItems: []knowledge.KnowledgeVersion[knowledge.Entity]{version},
	}
	similarity := &similarityIndex{}
	searcher := newTestSearcher(t, identities, entities, similarity)

	result, err := searcher.Search(t.Context(), Query{
		Title: " ALICE ",
		Type:  " PERSON ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if identities.findCalls != 1 || identities.title != "ALICE" || identities.entityType != "PERSON" {
		t.Fatalf("identity search = calls %d, title %q, type %q", identities.findCalls, identities.title, identities.entityType)
	}
	if similarity.calls != 0 {
		t.Fatalf("semantic search calls = %d, want 0", similarity.calls)
	}
	if len(result.Entities) != 1 ||
		result.Entities[0].Reason != "exact_identity" ||
		result.Entities[0].Version.Version != version.Version ||
		result.Entities[0].Version.Knowledge.ID != version.Knowledge.ID {
		t.Fatalf("Entity matches = %#v", result.Entities)
	}
	if len(entities.readReferences) != 1 || entities.readReferences[0] != (knowledge.Reference[knowledge.EntityID]{
		ID: searchEntityID, Version: 3,
	}) {
		t.Fatalf("exact Entity read = %#v", entities.readReferences)
	}
}

func TestSearcherFallsBackToSemanticSearchAfterExactMiss(t *testing.T) {
	version := searchEntityVersion()
	reference := knowledge.Reference[knowledge.EntityID]{ID: searchEntityID, Version: version.Version}
	identities := &identityIndex{}
	entities := &versions[knowledge.Entity, knowledge.EntityID]{
		activeItems: []knowledge.Reference[knowledge.EntityID]{reference},
		readItems:   []knowledge.KnowledgeVersion[knowledge.Entity]{version},
	}
	similarity := &similarityIndex{
		matches: []EntitySimilarity{{Reference: reference, Score: 0.91}},
	}
	searcher := newTestSearcher(t, identities, entities, similarity)

	result, err := searcher.Search(t.Context(), Query{Title: "Alice", Type: "Person"})
	if err != nil {
		t.Fatal(err)
	}
	if identities.findCalls != 1 {
		t.Fatalf("identity search calls = %d, want 1", identities.findCalls)
	}
	if similarity.calls != 1 || similarity.query.Text != "Alice Person" {
		t.Fatalf("semantic search = calls %d, query %#v", similarity.calls, similarity.query)
	}
	if len(result.Entities) != 1 || result.Entities[0].Reason != "semantic_match" || result.Entities[0].Score != 0.91 {
		t.Fatalf("Entity matches = %#v", result.Entities)
	}
}

func TestSearcherFuzzySearchSkipsExactIdentity(t *testing.T) {
	version := searchEntityVersion()
	reference := knowledge.Reference[knowledge.EntityID]{ID: searchEntityID, Version: version.Version}
	identities := &identityIndex{}
	entities := &versions[knowledge.Entity, knowledge.EntityID]{
		activeItems: []knowledge.Reference[knowledge.EntityID]{reference},
		readItems:   []knowledge.KnowledgeVersion[knowledge.Entity]{version},
	}
	similarity := &similarityIndex{
		matches: []EntitySimilarity{{Reference: reference, Score: 0.75}},
	}
	searcher := newTestSearcher(t, identities, entities, similarity)

	if _, err := searcher.Search(t.Context(), Query{Title: "someone at acme", Fuzzy: true}); err != nil {
		t.Fatal(err)
	}
	if identities.findCalls != 0 {
		t.Fatalf("identity search calls = %d, want 0", identities.findCalls)
	}
	if similarity.calls != 1 {
		t.Fatalf("semantic search calls = %d, want 1", similarity.calls)
	}
}

func searchEntityVersion() knowledge.KnowledgeVersion[knowledge.Entity] {
	return knowledge.KnowledgeVersion[knowledge.Entity]{
		Knowledge: knowledge.Entity{
			ID:          searchEntityID,
			Title:       "ALICE",
			Type:        "PERSON",
			Aliases:     []string{"Alice"},
			Description: "Alice is a person.",
		},
		Version: 3,
	}
}

func newTestSearcher(
	t *testing.T,
	identities *identityIndex,
	entities *versions[knowledge.Entity, knowledge.EntityID],
	similarity *similarityIndex,
) *Searcher {
	t.Helper()
	searcher, err := NewSearcher(SearchDependencies{
		Knowledge: knowledgeSource{view: &knowledgeView{
			identities: identities,
			entities:   entities,
			relations:  &versions[knowledge.Relation, knowledge.RelationID]{},
			claims:     &versions[knowledge.Claim, knowledge.ClaimID]{},
		}},
		Evidence: evidenceReader{},
		Entities: similarity,
		Corpora:  corporaReader{},
		Messages: messageReader{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return searcher
}

type knowledgeSource struct {
	view knowledge.View
}

func (source knowledgeSource) OpenCurrent(context.Context) (knowledge.View, error) {
	return source.view, nil
}

type knowledgeView struct {
	identities knowledge.IdentityReader
	entities   knowledge.VersionReader[knowledge.Entity, knowledge.EntityID]
	relations  knowledge.VersionReader[knowledge.Relation, knowledge.RelationID]
	claims     knowledge.VersionReader[knowledge.Claim, knowledge.ClaimID]
}

func (view *knowledgeView) Identities() knowledge.IdentityReader {
	return view.identities
}

func (view *knowledgeView) Entities() knowledge.VersionReader[knowledge.Entity, knowledge.EntityID] {
	return view.entities
}

func (view *knowledgeView) Relations() knowledge.VersionReader[knowledge.Relation, knowledge.RelationID] {
	return view.relations
}

func (view *knowledgeView) Claims() knowledge.VersionReader[knowledge.Claim, knowledge.ClaimID] {
	return view.claims
}

func (*knowledgeView) Close() error {
	return nil
}

type identityIndex struct {
	entities   knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID]
	findCalls  int
	title      string
	entityType string
}

func (*identityIndex) Entity(context.Context, knowledge.EntityIdentity) (knowledge.KnowledgeVersion[knowledge.Entity], bool, error) {
	return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, nil
}

func (index *identityIndex) FindEntities(
	_ context.Context,
	title string,
	entityType string,
	_ int,
) (knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID], error) {
	index.findCalls++
	index.title = title
	index.entityType = entityType
	return index.entities, nil
}

func (*identityIndex) Relation(context.Context, knowledge.RelationKey) (knowledge.KnowledgeVersion[knowledge.Relation], bool, error) {
	return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, nil
}

func (*identityIndex) Claim(context.Context, knowledge.ClaimIdentity) (knowledge.KnowledgeVersion[knowledge.Claim], bool, error) {
	return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, nil
}

type versions[T knowledge.Knowledge, ID knowledge.KnowledgeID] struct {
	activeItems    []knowledge.Reference[ID]
	currentItems   []knowledge.KnowledgeVersion[T]
	readItems      []knowledge.KnowledgeVersion[T]
	readReferences []knowledge.Reference[ID]
}

func (versions *versions[T, ID]) Active(
	context.Context,
	ID,
	int,
) (knowledge.Page[knowledge.Reference[ID], ID], error) {
	return knowledge.Page[knowledge.Reference[ID], ID]{Items: versions.activeItems}, nil
}

func (versions *versions[T, ID]) Current(
	context.Context,
	ID,
	int,
) (knowledge.Page[knowledge.KnowledgeVersion[T], ID], error) {
	return knowledge.Page[knowledge.KnowledgeVersion[T], ID]{Items: versions.currentItems}, nil
}

func (*versions[T, ID]) History(
	context.Context,
	ID,
	knowledge.Version,
	int,
) (knowledge.Page[knowledge.KnowledgeVersion[T], knowledge.Version], error) {
	return knowledge.Page[knowledge.KnowledgeVersion[T], knowledge.Version]{}, nil
}

func (versions *versions[T, ID]) Read(
	_ context.Context,
	references []knowledge.Reference[ID],
) ([]knowledge.KnowledgeVersion[T], error) {
	versions.readReferences = append([]knowledge.Reference[ID](nil), references...)
	return versions.readItems, nil
}

type similarityIndex struct {
	calls   int
	query   EntityQuery
	matches []EntitySimilarity
}

func (index *similarityIndex) MatchEntities(_ context.Context, query EntityQuery) ([]EntitySimilarity, error) {
	index.calls++
	index.query = query
	return index.matches, nil
}

type evidenceReader struct{}

func (evidenceReader) ReadEntityEvidence(context.Context, knowledge.Reference[knowledge.EntityID]) ([]provenance.EntityConfirmation, error) {
	return nil, nil
}

func (evidenceReader) ReadRelationEvidence(context.Context, knowledge.Reference[knowledge.RelationID]) ([]provenance.RelationConfirmation, error) {
	return nil, nil
}

func (evidenceReader) ReadClaimEvidence(context.Context, knowledge.Reference[knowledge.ClaimID]) ([]provenance.ClaimConfirmation, error) {
	return nil, nil
}

type corporaReader struct{}

func (corporaReader) TextUnitLocations(context.Context, corpus.CorporaID, []textunits.TextUnitID) ([]corpus.TextUnitLocation, error) {
	return nil, nil
}

type messageReader struct{}

func (messageReader) Read(context.Context, []string) ([]message.Occurrence, error) {
	return nil, nil
}
