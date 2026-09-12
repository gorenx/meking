package graph

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCatalogBrowsesOneCurrentKnowledgeGraph(t *testing.T) {
	view := &fakeKnowledgeView{
		entities: []KnowledgeEntity{
			{ID: "entity-b", Version: 2, Title: "Beta", Type: "org", Degree: 7, TextUnitCount: 2},
			{ID: "entity-a", Version: 3, Title: "Alpha", Type: "person", Aliases: []string{"A"}, Degree: 9, TextUnitCount: 1},
			{ID: "entity-c", Version: 1, Title: "Gamma", Type: "org", Degree: 3},
			{ID: "entity-deleted", Version: 4, Deleted: true, Title: "Deleted", Degree: 99},
		},
		relations: []KnowledgeRelation{
			{ID: "relation-ab", Version: 2, SourceEntityID: "entity-a", TargetEntityID: "entity-b", CombinedDegree: 16, Weight: 2},
			{ID: "relation-ac", Version: 1, SourceEntityID: "entity-a", TargetEntityID: "entity-c", CombinedDegree: 12, Weight: 1},
			{ID: "relation-deleted", Deleted: true, SourceEntityID: "entity-a", TargetEntityID: "entity-b"},
		},
	}
	knowledge := &fakeKnowledgeReader{view: view}
	structures := &fakeStructureReader{err: errors.New("Community Structure must not be read")}
	catalog := mustCatalog(t, knowledge, structures)

	result, err := catalog.BrowseEntities(context.Background(), EntityGraphRequest{Limit: 2})
	if err != nil {
		t.Fatalf("BrowseEntities() error = %v", err)
	}
	if knowledge.opens != 1 || !view.closed {
		t.Fatalf("fixed reads = opens %d/closed %t", knowledge.opens, view.closed)
	}
	if structures.calls != 0 {
		t.Fatalf("Entity graph read Community Structure %d times", structures.calls)
	}
	if got := entityIDs(result.Entities); !reflect.DeepEqual(got, []string{"entity-a", "entity-b"}) {
		t.Fatalf("Entities = %v", got)
	}
	if result.Entities[0].Version != 3 {
		t.Fatalf("first Entity = %#v", result.Entities[0])
	}
	if len(result.Relations) != 1 || result.Relations[0].ID != "relation-ab" ||
		result.MatchedEntities != 3 || result.MatchedRelations != 1 || !result.Truncated {
		t.Fatalf("graph = %#v", result)
	}
}

func TestCatalogFiltersEntitiesAndReadsBoundedNeighborhood(t *testing.T) {
	view := &fakeKnowledgeView{
		entities: []KnowledgeEntity{
			{ID: "entity-a", Title: "Alpha", Type: "person", Aliases: []string{"Leader"}, Degree: 9},
			{ID: "entity-b", Title: "Beta", Type: "org", Degree: 7},
			{ID: "entity-c", Title: "Gamma", Type: "org", Degree: 3},
		},
		relations: []KnowledgeRelation{
			{ID: "relation-low", SourceEntityID: "entity-a", TargetEntityID: "entity-c", CombinedDegree: 12},
			{ID: "relation-high", SourceEntityID: "entity-b", TargetEntityID: "entity-a", CombinedDegree: 16},
		},
	}
	catalog := mustCatalog(t,
		&fakeKnowledgeReader{view: view}, &fakeStructureReader{structure: testStructure()},
	)

	filtered, err := catalog.BrowseEntities(context.Background(), EntityGraphRequest{
		Query: "leader", Limit: 10,
	})
	if err != nil {
		t.Fatalf("BrowseEntities() error = %v", err)
	}
	if got := entityIDs(filtered.Entities); !reflect.DeepEqual(got, []string{"entity-a"}) {
		t.Fatalf("filtered Entities = %v", got)
	}

	view.closed = false
	neighbors, err := catalog.Neighborhood(context.Background(), NeighborhoodRequest{
		EntityID: "entity-a", RelationLimit: 1,
	})
	if err != nil {
		t.Fatalf("Neighborhood() error = %v", err)
	}
	if neighbors.Center.ID != "entity-a" ||
		!reflect.DeepEqual(entityIDs(neighbors.Entities), []string{"entity-a", "entity-b"}) ||
		len(neighbors.Relations) != 1 || neighbors.Relations[0].ID != "relation-high" ||
		neighbors.MatchedRelations != 2 || !neighbors.Truncated || !view.closed {
		t.Fatalf("Neighborhood = %#v", neighbors)
	}
}

func TestCatalogBrowsesCommunityRootsChildrenAndSearch(t *testing.T) {
	catalog := mustCatalog(t,
		&fakeKnowledgeReader{view: &fakeKnowledgeView{}},
		&fakeStructureReader{structure: testStructure()},
	)
	root, err := catalog.BrowseCommunities(context.Background(), CommunityRequest{})
	if err != nil {
		t.Fatalf("BrowseCommunities(root) error = %v", err)
	}
	if root.Total != 1 || len(root.Communities) != 1 || root.Communities[0].ID != "community-root" ||
		root.Communities[0].ChildCount != 1 || root.StructureID != "structure-1" {
		t.Fatalf("root page = %#v", root)
	}
	parent := "community-root"
	children, err := catalog.BrowseCommunities(context.Background(), CommunityRequest{ParentID: &parent})
	if err != nil {
		t.Fatalf("BrowseCommunities(children) error = %v", err)
	}
	if children.Total != 1 || children.Communities[0].ID != "community-child" {
		t.Fatalf("children = %#v", children)
	}
	search, err := catalog.BrowseCommunities(context.Background(), CommunityRequest{Query: "child"})
	if err != nil || search.Total != 1 || search.Communities[0].ID != "community-child" {
		t.Fatalf("search/error = %#v/%v", search, err)
	}
}

func TestCatalogRejectsInvalidAndMissingGraphSelections(t *testing.T) {
	catalog := mustCatalog(t,
		&fakeKnowledgeReader{view: &fakeKnowledgeView{}},
		&fakeStructureReader{structure: testStructure()},
	)
	if _, err := catalog.BrowseEntities(context.Background(), EntityGraphRequest{Limit: MaximumEntityLimit + 1}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid Entity request error = %v", err)
	}
	if _, err := catalog.Neighborhood(context.Background(), NeighborhoodRequest{EntityID: "missing"}); !errors.Is(err, ErrEntityNotFound) {
		t.Fatalf("missing Entity error = %v", err)
	}
	missing := "missing"
	if _, err := catalog.BrowseCommunities(context.Background(), CommunityRequest{ParentID: &missing}); !errors.Is(err, ErrCommunityNotFound) {
		t.Fatalf("missing Community error = %v", err)
	}
}

func TestKnowledgeReadsFollowExplicitPageBoundary(t *testing.T) {
	view := &fakeKnowledgeView{
		entities:  []KnowledgeEntity{{ID: "entity-a"}, {ID: "entity-b"}},
		relations: []KnowledgeRelation{{ID: "relation-a"}, {ID: "relation-b"}},
		pageSize:  1,
	}
	entities, err := readEntities(t.Context(), view)
	if err != nil {
		t.Fatalf("readEntities() error = %v", err)
	}
	relations, err := readRelations(t.Context(), view)
	if err != nil {
		t.Fatalf("readRelations() error = %v", err)
	}
	if len(entities) != 2 || entities[0].ID != "entity-a" || entities[1].ID != "entity-b" {
		t.Fatalf("Entities = %#v", entities)
	}
	if len(relations) != 2 || relations[0].ID != "relation-a" || relations[1].ID != "relation-b" {
		t.Fatalf("Relations = %#v", relations)
	}
}

func testStructure() Structure {
	parent := "community-root"
	return Structure{
		ID: "structure-1", CommunitySetID: "communities-1", CorporaID: "corpus-1",
		Memberships: []Membership{
			{ID: "community-root", Number: 0, Level: 0, EntityIDs: []string{"entity-a", "entity-b", "entity-c"}},
			{ID: "community-child", Number: 1, Level: 1, ParentID: &parent, EntityIDs: []string{"entity-a", "entity-b"}},
		},
	}
}

func mustCatalog(t *testing.T, knowledge KnowledgeReader, structures StructureReader) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(knowledge, structures)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	return catalog
}

type fakeKnowledgeReader struct {
	view  KnowledgeView
	opens int
}

func (r *fakeKnowledgeReader) OpenCurrent(context.Context) (KnowledgeView, error) {
	r.opens++
	return r.view, nil
}

type fakeKnowledgeView struct {
	entities  []KnowledgeEntity
	relations []KnowledgeRelation
	pageSize  int
	closed    bool
}

func (v *fakeKnowledgeView) Entities(_ context.Context, after string, limit int) (KnowledgePage[KnowledgeEntity], error) {
	return entityPage(v.entities, after, v.limit(limit)), nil
}

func (v *fakeKnowledgeView) Relations(_ context.Context, after string, limit int) (KnowledgePage[KnowledgeRelation], error) {
	limit = v.limit(limit)
	start := 0
	for start < len(v.relations) && v.relations[start].ID <= after {
		start++
	}
	end := min(start+limit, len(v.relations))
	page := KnowledgePage[KnowledgeRelation]{Items: append([]KnowledgeRelation(nil), v.relations[start:end]...)}
	if end < len(v.relations) {
		page.NextAfter = page.Items[len(page.Items)-1].ID
		page.HasMore = true
	}
	return page, nil
}

func (v *fakeKnowledgeView) Close() error {
	v.closed = true
	return nil
}

func (v *fakeKnowledgeView) limit(requested int) int {
	if v.pageSize > 0 && v.pageSize < requested {
		return v.pageSize
	}
	return requested
}

func entityPage(values []KnowledgeEntity, after string, limit int) KnowledgePage[KnowledgeEntity] {
	start := 0
	for start < len(values) && values[start].ID <= after {
		start++
	}
	end := min(start+limit, len(values))
	page := KnowledgePage[KnowledgeEntity]{Items: append([]KnowledgeEntity(nil), values[start:end]...)}
	if end < len(values) {
		page.NextAfter = page.Items[len(page.Items)-1].ID
		page.HasMore = true
	}
	return page
}

type fakeStructureReader struct {
	structure Structure
	err       error
	calls     int
}

func (r *fakeStructureReader) Current(context.Context) (Structure, error) {
	r.calls++
	return r.structure, r.err
}

func entityIDs(values []Entity) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.ID
	}
	return result
}
