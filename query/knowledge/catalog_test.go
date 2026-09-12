package knowledge

import (
	"context"
	"errors"
	"testing"

	domain "github.com/memoria-space/meking/knowledge"
	querybase "github.com/memoria-space/meking/query"
	querysource "github.com/memoria-space/meking/query/source"
)

func TestCatalogBrowsesActiveEntitiesAtOneEpochBoundary(t *testing.T) {
	epochs := &entityEpochReader{epoch: querybase.Epoch{
		ID: 7, CorporaID: "corpus-1", ReportSetID: "reports-1",
	}}
	view := &entityView{page: Page[Entity]{
		Items:     []Entity{{ID: "entity-1", Version: 3, Title: "Alpha", Aliases: []string{}}},
		NextAfter: "entity-1", HasMore: true,
	}}
	reader := &entityReader{view: view}
	catalog, err := NewCatalog(epochs, reader, &entitySource{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := catalog.BrowseEntities(t.Context(), PageRequest{After: " previous ", Limit: 12})
	if err != nil {
		t.Fatal(err)
	}
	if epochs.calls != 1 || !reader.versions.Equal(epochs.epoch.Knowledge) || !view.closed ||
		view.after != "previous" || view.limit != 12 {
		t.Fatalf("fixed read = epochs:%d versions:%#v closed:%t request:%q/%d",
			epochs.calls, reader.versions, view.closed, view.after, view.limit)
	}
	if page.EpochID != 7 || page.CorporaID != "corpus-1" ||
		page.ReportSetID != "reports-1" || len(page.Items) != 1 || page.Items[0].Version != 3 ||
		page.Items[0].Aliases == nil || page.NextAfter != "entity-1" || !page.HasMore {
		t.Fatalf("Entity page = %#v", page)
	}
}

func TestCatalogBrowsesActiveClaimsAtOneEpochBoundary(t *testing.T) {
	epochs := &entityEpochReader{epoch: querybase.Epoch{
		ID: 7, CorporaID: "corpus-1", ReportSetID: "reports-1",
	}}
	view := &entityView{claimPage: Page[Claim]{
		Items: []Claim{{
			ID: "claim-1", Version: 2, SubjectID: "entity-1", Type: "status",
			Evidence: []ClaimEvidence{{TextUnitID: "unit-1", SourceText: "source"}},
		}},
		NextAfter: "claim-1", HasMore: true,
	}}
	catalog, err := NewCatalog(epochs, &entityReader{view: view}, &entitySource{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := catalog.BrowseClaims(t.Context(), PageRequest{After: " previous ", Limit: 12})
	if err != nil {
		t.Fatal(err)
	}
	if epochs.calls != 1 || !view.closed || view.claimAfter != "previous" || view.claimLimit != 12 ||
		page.EpochID != 7 || len(page.Items) != 1 ||
		page.Items[0].SubjectID != "entity-1" || len(page.Items[0].Evidence) != 1 ||
		page.NextAfter != "claim-1" || !page.HasMore {
		t.Fatalf("Claim page = %#v; fixed read = %d/%t/%q/%d",
			page, epochs.calls, view.closed, view.claimAfter, view.claimLimit)
	}
}

func TestCatalogRejectsInvalidPageAndMissingEpoch(t *testing.T) {
	catalog, err := NewCatalog(
		&entityEpochReader{err: querybase.ErrNoEpoch},
		&entityReader{view: &entityView{}},
		&entitySource{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BrowseEntities(t.Context(), PageRequest{Limit: MaximumPageSize + 1}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid page error = %v", err)
	}
	if _, err := catalog.BrowseEntities(t.Context(), PageRequest{}); !errors.Is(err, ErrNoPublication) {
		t.Fatalf("missing Epoch error = %v", err)
	}
}

func TestCatalogReadsEntityAndRelationAssociationClosureAtOneEpoch(t *testing.T) {
	view := &entityView{
		page: Page[Entity]{Items: []Entity{
			{ID: "entity-1", Version: 2, Title: "Alpha", TextUnitIDs: []string{"unit-1"}},
			{ID: "entity-2", Version: 1, Title: "Beta"},
		}},
		relationPage: Page[Relation]{Items: []Relation{{
			ID: "relation-1", Version: 3, SourceEntityID: "entity-1", TargetEntityID: "entity-2",
			TextUnitIDs: []string{"unit-2"},
		}}},
		claimPage: Page[Claim]{Items: []Claim{{
			ID: "claim-1", Version: 1, SubjectID: "entity-1", Type: "status",
		}}},
	}
	sources := &entitySource{values: []querysource.TextUnitSource{{
		CorporaID: "corpus-1", TextUnitID: "unit-1", Text: "evidence",
	}}}
	catalog, err := NewCatalog(
		&entityEpochReader{epoch: querybase.Epoch{
			ID: 7, CorporaID: "corpus-1", ReportSetID: "reports-1",
		}},
		&entityReader{view: view},
		sources,
	)
	if err != nil {
		t.Fatal(err)
	}
	entity, err := catalog.Entity(t.Context(), "entity-1", 7)
	if err != nil {
		t.Fatal(err)
	}
	if entity.EpochID != 7 || entity.Entity.Version != 2 || len(entity.Relations) != 1 ||
		len(entity.Neighbors) != 1 || entity.Neighbors[0].ID != "entity-2" ||
		len(entity.Claims) != 1 || len(entity.TextUnits) != 1 ||
		view.limit != MaximumPageSize || view.relationLimit != MaximumPageSize ||
		view.claimLimit != MaximumPageSize ||
		sources.corporaID != "corpus-1" || len(sources.ids) != 1 || sources.ids[0] != "unit-1" {
		t.Fatalf("Entity detail = %#v; source = %#v", entity, sources)
	}
	sources.values[0].TextUnitID = "unit-2"
	relation, err := catalog.Relation(t.Context(), "relation-1", 7)
	if err != nil {
		t.Fatal(err)
	}
	if relation.Relation.Version != 3 || len(relation.Endpoints) != 2 ||
		len(relation.Claims) != 0 || len(relation.TextUnits) != 1 || sources.ids[0] != "unit-2" {
		t.Fatalf("Relation detail = %#v; source = %#v", relation, sources)
	}
	if epochs := catalog.epochs.(*entityEpochReader); epochs.calls != 0 || epochs.exactCalls != 2 || epochs.exactID != 7 {
		t.Fatalf("Epoch reads = Current %d, exact %d/%d", epochs.calls, epochs.exactCalls, epochs.exactID)
	}
}

type entityEpochReader struct {
	epoch      querybase.Epoch
	err        error
	calls      int
	exactCalls int
	exactID    int64
}

func (r *entityEpochReader) Current(context.Context) (querybase.Epoch, error) {
	r.calls++
	return r.epoch, r.err
}

func (r *entityEpochReader) Epoch(_ context.Context, id int64) (querybase.Epoch, error) {
	r.exactCalls++
	r.exactID = id
	if r.err != nil {
		return querybase.Epoch{}, r.err
	}
	if id != r.epoch.ID {
		return querybase.Epoch{}, querybase.ErrEpochNotFound
	}
	return r.epoch, nil
}

type entityReader struct {
	view     View
	versions domain.Manifest
}

func (r *entityReader) Open(_ context.Context, versions domain.Manifest) (View, error) {
	r.versions = versions.Clone()
	return r.view, nil
}

type entityView struct {
	page          Page[Entity]
	relationPage  Page[Relation]
	claimPage     Page[Claim]
	claimAfter    string
	claimLimit    int
	relationLimit int
	after         string
	limit         int
	closed        bool
}

func (v *entityView) Relations(_ context.Context, _ string, limit int) (Page[Relation], error) {
	v.relationLimit = limit
	return v.relationPage, nil
}

func (v *entityView) Claims(_ context.Context, after string, limit int) (Page[Claim], error) {
	v.claimAfter, v.claimLimit = after, limit
	return v.claimPage, nil
}

func (v *entityView) Entities(_ context.Context, after string, limit int) (Page[Entity], error) {
	v.after, v.limit = after, limit
	return v.page, nil
}

func (v *entityView) Close() error {
	v.closed = true
	return nil
}

type entitySource struct {
	corporaID string
	ids       []string
	values    []querysource.TextUnitSource
}

func (s *entitySource) Read(_ context.Context, corporaID string, ids []string) ([]querysource.TextUnitSource, error) {
	s.corporaID = corporaID
	s.ids = append([]string(nil), ids...)
	return append([]querysource.TextUnitSource(nil), s.values...), nil
}
