package report

import (
	"context"
	"errors"
	"reflect"
	"testing"

	querybase "github.com/memoria-space/meking/query"
)

// reportPublicationStub records publication reads while returning an isolated
// fixture controlled by each Report Catalog test.
type reportPublicationStub struct {
	publication ReportPublication
	err         error
	calls       int
	reportSetID string
}

func (s *reportPublicationStub) ReportPublication(
	_ context.Context,
	reportSetID string,
) (ReportPublication, error) {
	s.calls++
	s.reportSetID = reportSetID
	return s.publication, s.err
}

type reportEpochStub struct {
	current    querybase.Epoch
	err        error
	calls      int
	exactCalls int
	exactID    int64
}

func (s *reportEpochStub) Current(context.Context) (querybase.Epoch, error) {
	s.calls++
	return s.current, s.err
}

func (s *reportEpochStub) Epoch(_ context.Context, id int64) (querybase.Epoch, error) {
	s.exactCalls++
	s.exactID = id
	if s.err != nil {
		return querybase.Epoch{}, s.err
	}
	if id != s.current.ID {
		return querybase.Epoch{}, querybase.ErrEpochNotFound
	}
	return s.current, nil
}

func TestReportCatalogBrowsesCurrentCompletePublication(t *testing.T) {
	publication := reportPublicationFixture()
	reader := &reportPublicationStub{publication: publication}
	catalog := newReportCatalogForTest(t, reader)

	page, err := catalog.Browse(t.Context(), ReportPageRequest{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("Browse() error = %v", err)
	}
	if page.ReportSetID != publication.ReportSetID || page.CommunitySetID != publication.CommunitySetID ||
		page.CorporaID != publication.CorporaID || page.Total != 3 || len(page.Reports) != 2 ||
		page.Reports[0].ID != "report-root" || page.Reports[1].ID != "report-a" ||
		page.Reports[0].CommunityNumber != 0 || page.Reports[0].Size != 2 {
		t.Fatalf("Browse() = %#v", page)
	}
	if reader.calls != 1 || reader.reportSetID != publication.ReportSetID {
		t.Fatalf("publication reads = %d for %q", reader.calls, reader.reportSetID)
	}
}

func TestReportCatalogReadsPublishedSourcesWithoutConsultingKnowledgeCurrent(t *testing.T) {
	publication := reportPublicationFixture()
	catalog := newReportCatalogForTest(t, &reportPublicationStub{publication: publication})

	detail, err := catalog.Read(t.Context(), "report-a", 7)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if detail.ID != "report-a" || detail.CommunityID != "community-a" ||
		detail.CommunityNumber != 1 || detail.ParentID == nil || *detail.ParentID != "community-root" ||
		len(detail.Children) != 0 || detail.Sources.Entities[0].Version != 3 ||
		detail.Sources.Claims[0].Version != 2 {
		t.Fatalf("Read() = %#v", detail)
	}
	publication.Reports[1].Sources.Entities[0].ID = "mutated"
	if detail.Sources.Entities[0].ID != "entity-a" {
		t.Fatalf("detail shares source storage with provider: %#v", detail.Sources)
	}
	if epochs := catalog.epochs.(*reportEpochStub); epochs.calls != 0 || epochs.exactCalls != 1 || epochs.exactID != 7 {
		t.Fatalf("Epoch reads = Current %d, exact %d/%d", epochs.calls, epochs.exactCalls, epochs.exactID)
	}
}

func TestReportCatalogReturnsIsolatedCurrentView(t *testing.T) {
	publication := reportPublicationFixture()
	reader := &reportPublicationStub{publication: publication}
	catalog := newReportCatalogForTest(t, reader)

	view, err := catalog.Current(t.Context())
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if view.ReportSetID != publication.ReportSetID ||
		view.EpochID != 7 ||
		view.CommunitySetID != publication.CommunitySetID ||
		view.CorporaID != publication.CorporaID ||
		len(view.Communities) != len(publication.Communities) ||
		len(view.Reports) != len(publication.Reports) {
		t.Fatalf("Current() = %#v", view)
	}
	reader.publication.Communities[0].EntityIDs[0] = "mutated"
	reader.publication.Reports[0].Sources.Entities[0].ID = "mutated"
	if view.Communities[0].EntityIDs[0] != "entity-a" ||
		view.Reports[0].Sources.Entities[0].ID != "entity-a" {
		t.Fatalf("Current() shares provider storage: %#v", view)
	}
}

func TestReportReaderLoadsExactEpochWithoutCurrentSelection(t *testing.T) {
	publication := reportPublicationFixture()
	provider := &reportPublicationStub{publication: publication}
	reader, err := NewReader(provider)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	selected := querybase.Epoch{
		ID:        9,
		CorporaID: publication.CorporaID, ReportSetID: publication.ReportSetID,
	}
	view, err := reader.Publication(t.Context(), selected)
	if err != nil {
		t.Fatalf("Publication() error = %v", err)
	}
	if view.EpochID != selected.ID || view.ReportSetID != selected.ReportSetID ||
		provider.calls != 1 || provider.reportSetID != selected.ReportSetID {
		t.Fatalf("View/provider = %#v / %#v", view, provider)
	}
}

func TestReportCatalogRejectsInvalidRequestsBeforeReadingPublication(t *testing.T) {
	reader := &reportPublicationStub{}
	catalog := newReportCatalogForTest(t, reader)
	if _, err := catalog.Browse(
		t.Context(),
		ReportPageRequest{Page: 0, PageSize: 10},
	); !errors.Is(err, ErrInvalidReportRequest) {
		t.Fatalf("Browse() error = %v, want ErrInvalidReportRequest", err)
	}
	if _, err := catalog.Read(t.Context(), " ", 7); !errors.Is(err, ErrInvalidReportRequest) {
		t.Fatalf("Read() error = %v, want ErrInvalidReportRequest", err)
	}
	if _, err := catalog.Read(t.Context(), "report-a", 0); !errors.Is(err, ErrInvalidReportRequest) {
		t.Fatalf("Read() invalid Epoch error = %v, want ErrInvalidReportRequest", err)
	}
	if reader.calls != 0 {
		t.Fatalf("invalid request read publication %d times", reader.calls)
	}
}

func TestReportCatalogReturnsMissingReport(t *testing.T) {
	publication := reportPublicationFixture()
	catalog := newReportCatalogForTest(t, &reportPublicationStub{publication: publication})
	if _, err := catalog.Read(t.Context(), "missing", 7); !errors.Is(err, ErrReportNotFound) {
		t.Fatalf("Read() error = %v, want ErrReportNotFound", err)
	}
}

func TestReportCatalogReturnsEmptyPageBeyondPublication(t *testing.T) {
	publication := reportPublicationFixture()
	catalog := newReportCatalogForTest(t, &reportPublicationStub{publication: publication})
	page, err := catalog.Browse(t.Context(), ReportPageRequest{
		Page: int(^uint(0) >> 1), PageSize: MaximumReportPageSize,
	})
	if err != nil {
		t.Fatalf("Browse() error = %v", err)
	}
	if page.Total != len(publication.Reports) || len(page.Reports) != 0 {
		t.Fatalf("Browse() = %#v", page)
	}
}

func TestNewReportCatalogRequiresPublicationReader(t *testing.T) {
	reader := &reportPublicationStub{publication: reportPublicationFixture()}
	if _, err := NewReportCatalog(nil, reader); err == nil {
		t.Fatal("NewReportCatalog(nil, reader) error = nil")
	}
	if _, err := NewReportCatalog(defaultReportEpoch(), nil); err == nil {
		t.Fatal("NewReportCatalog(epoch, nil) error = nil")
	}
	catalog := newReportCatalogForTest(t, reader)
	page, err := catalog.Browse(t.Context(), ReportPageRequest{Page: 1, PageSize: 3})
	if err != nil || !reflect.DeepEqual(
		[]string{page.Reports[0].ID, page.Reports[1].ID, page.Reports[2].ID},
		[]string{"report-root", "report-a", "report-b"},
	) {
		t.Fatalf("Browse() Reports/error = %#v/%v", page.Reports, err)
	}
}

func TestReportCatalogRequiresPublishedEpoch(t *testing.T) {
	reader := &reportPublicationStub{publication: reportPublicationFixture()}
	epochs := &reportEpochStub{err: querybase.ErrNoEpoch}
	catalog, err := NewReportCatalog(epochs, reader)
	if err != nil {
		t.Fatalf("NewReportCatalog() error = %v", err)
	}
	if _, err := catalog.Current(t.Context()); !errors.Is(err, ErrNoReportPublication) {
		t.Fatalf("Current() error = %v, want ErrNoReportPublication", err)
	}
	if epochs.calls != 1 || reader.calls != 0 {
		t.Fatalf("Epoch/publication reads = %d/%d, want 1/0", epochs.calls, reader.calls)
	}
}

func newReportCatalogForTest(
	t *testing.T,
	publications ReportPublicationReader,
) *ReportCatalog {
	t.Helper()
	catalog, err := NewReportCatalog(defaultReportEpoch(), publications)
	if err != nil {
		t.Fatalf("NewReportCatalog() error = %v", err)
	}
	return catalog
}

func defaultReportEpoch() *reportEpochStub {
	return &reportEpochStub{current: querybase.Epoch{
		ID:        7,
		CorporaID: "corpora", ReportSetID: "report-set",
	}}
}

func reportPublicationFixture() ReportPublication {
	root := "community-root"
	return ReportPublication{
		ReportSetID: "report-set", CommunitySetID: "community-set", CorporaID: "corpora",
		Communities: []PublishedCommunity{
			{ID: root, Number: 0, Level: 0, EntityIDs: []string{"entity-a", "entity-b"}},
			{ID: "community-a", Number: 1, Level: 1, ParentID: &root, EntityIDs: []string{"entity-a"}},
			{ID: "community-b", Number: 2, Level: 1, ParentID: &root, EntityIDs: []string{"entity-b"}},
		},
		Reports: []PublishedReport{
			reportFixture("report-root", root, ReportSources{
				Entities: []querybase.KnowledgeReference{
					{ID: "entity-a", Version: 3},
					{ID: "entity-b", Version: 2},
				},
				TextUnitIDs: []string{"text-1", "text-2"},
			}),
			reportFixture("report-a", "community-a", ReportSources{
				Entities:    []querybase.KnowledgeReference{{ID: "entity-a", Version: 3}},
				Claims:      []querybase.ClaimReference{{ID: "claim-a", Version: 2, EvidenceIndex: 0}},
				TextUnitIDs: []string{"text-1"},
			}),
			reportFixture("report-b", "community-b", ReportSources{
				Entities:    []querybase.KnowledgeReference{{ID: "entity-b", Version: 2}},
				Relations:   []querybase.KnowledgeReference{{ID: "relation-b", Version: 1}},
				TextUnitIDs: []string{"text-2"},
			}),
		},
	}
}

func reportFixture(id string, communityID string, sources ReportSources) PublishedReport {
	return PublishedReport{
		ID: id, CommunityID: communityID, Period: "2026-07-25",
		Title: "Title " + id, Summary: "Summary " + id,
		Findings: []ReportFinding{{Summary: "Finding", Explanation: "Evidence"}},
		Rank:     7.5, RatingExplanation: "Rating", FullContent: "# " + id,
		Sources: sources,
	}
}
