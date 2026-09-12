package adapter

import (
	"context"
	"errors"
	"reflect"
	"testing"

	knowledgedomain "github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

type currentReports struct {
	view     queryreport.View
	err      error
	selected []querybase.Epoch
}

func (r *currentReports) Publication(
	_ context.Context,
	selected querybase.Epoch,
) (queryreport.View, error) {
	r.selected = append(r.selected, selected)
	return r.view, r.err
}

type knowledgeViews struct {
	view  knowledgedomain.View
	err   error
	opens int
}

func (s *knowledgeViews) OpenCurrent(context.Context) (knowledgedomain.View, error) {
	s.opens++
	return s.view, s.err
}

type entityVersions struct {
	requested []knowledgedomain.Reference[knowledgedomain.EntityID]
	versions  []knowledgedomain.KnowledgeVersion[knowledgedomain.Entity]
	err       error
}

func (*entityVersions) Active(
	context.Context,
	knowledgedomain.EntityID,
	int,
) (knowledgedomain.Page[knowledgedomain.Reference[knowledgedomain.EntityID], knowledgedomain.EntityID], error) {
	return knowledgedomain.Page[knowledgedomain.Reference[knowledgedomain.EntityID], knowledgedomain.EntityID]{}, errors.New("Active is not used")
}

func (*entityVersions) Current(
	context.Context,
	knowledgedomain.EntityID,
	int,
) (knowledgedomain.Page[knowledgedomain.KnowledgeVersion[knowledgedomain.Entity], knowledgedomain.EntityID], error) {
	return knowledgedomain.Page[knowledgedomain.KnowledgeVersion[knowledgedomain.Entity], knowledgedomain.EntityID]{}, errors.New("Current is not used")
}

func (*entityVersions) History(
	context.Context,
	knowledgedomain.EntityID,
	knowledgedomain.Version,
	int,
) (knowledgedomain.Page[knowledgedomain.KnowledgeVersion[knowledgedomain.Entity], knowledgedomain.Version], error) {
	return knowledgedomain.Page[knowledgedomain.KnowledgeVersion[knowledgedomain.Entity], knowledgedomain.Version]{}, errors.New("History is not used")
}

func (r *entityVersions) Read(
	_ context.Context,
	references []knowledgedomain.Reference[knowledgedomain.EntityID],
) ([]knowledgedomain.KnowledgeVersion[knowledgedomain.Entity], error) {
	r.requested = append([]knowledgedomain.Reference[knowledgedomain.EntityID](nil), references...)
	return r.versions, r.err
}

type fixedKnowledgeView struct {
	entities *entityVersions
	closes   int
}

func (*fixedKnowledgeView) Identities() knowledgedomain.IdentityReader { return nil }

func (v *fixedKnowledgeView) Entities() knowledgedomain.VersionReader[knowledgedomain.Entity, knowledgedomain.EntityID] {
	return v.entities
}

func (*fixedKnowledgeView) Relations() knowledgedomain.VersionReader[knowledgedomain.Relation, knowledgedomain.RelationID] {
	return nil
}

func (*fixedKnowledgeView) Claims() knowledgedomain.VersionReader[knowledgedomain.Claim, knowledgedomain.ClaimID] {
	return nil
}

func (v *fixedKnowledgeView) Close() error {
	v.closes++
	return nil
}

func TestEvidenceReaderReadsDistinctExactEntityVersions(t *testing.T) {
	reportView := queryreport.View{
		ReportSetID: "report-set", CommunitySetID: "community-set", CorporaID: "corpora",
		Reports: []queryreport.PublishedReport{
			{Sources: queryreport.ReportSources{Entities: []querybase.KnowledgeReference{
				{ID: "entity-b", Version: 2},
				{ID: "entity-a", Version: 3},
			}}},
			{Sources: queryreport.ReportSources{Entities: []querybase.KnowledgeReference{
				{ID: "entity-a", Version: 3},
				{ID: "entity-a", Version: 1},
			}}},
		},
	}
	versions := &entityVersions{versions: []knowledgedomain.KnowledgeVersion[knowledgedomain.Entity]{
		{Knowledge: knowledgedomain.Entity{ID: "entity-a"}, Version: 1},
		{Knowledge: knowledgedomain.Entity{ID: "entity-a"}, Version: 3},
		{Knowledge: knowledgedomain.Entity{ID: "entity-b"}, Version: 2},
	}}
	view := &fixedKnowledgeView{entities: versions}
	knowledge := &knowledgeViews{view: view}
	reports := &currentReports{view: reportView}
	metadata := entityMetadata{}
	reader, err := NewEvidenceReader(reports, knowledge, metadata)
	if err != nil {
		t.Fatalf("NewEvidenceReader() error = %v", err)
	}

	selected := querybase.Epoch{
		ID: 4, CorporaID: "corpora", ReportSetID: "report-set",
	}
	evidence, err := reader.ReportEvidence(t.Context(), selected)
	if err != nil {
		t.Fatalf("ReportEvidence() error = %v", err)
	}
	wantReferences := []knowledgedomain.Reference[knowledgedomain.EntityID]{
		{ID: "entity-a", Version: 1},
		{ID: "entity-a", Version: 3},
		{ID: "entity-b", Version: 2},
	}
	if !reflect.DeepEqual(versions.requested, wantReferences) {
		t.Fatalf("requested references = %#v, want %#v", versions.requested, wantReferences)
	}
	if evidence.ReportView.ReportSetID != "report-set" ||
		len(evidence.Entities) != 3 ||
		len(evidence.Entities[1].TextUnitIDs) != 1 || view.closes != 1 || knowledge.opens != 1 {
		t.Fatalf(
			"Report evidence = %#v; closes = %d; opens = %d",
			evidence, view.closes, knowledge.opens,
		)
	}
	if !reflect.DeepEqual(reports.selected, []querybase.Epoch{selected}) {
		t.Fatalf("selected Epochs = %#v", reports.selected)
	}
}

func TestEvidenceReaderDoesNotOpenKnowledgeWithoutVisibleReports(t *testing.T) {
	knowledge := &knowledgeViews{}
	reports := &currentReports{view: queryreport.View{
		ReportSetID: "report-set", CommunitySetID: "community-set", CorporaID: "corpora",
	}}
	reader, err := NewEvidenceReader(reports, knowledge, entityMetadata{})
	if err != nil {
		t.Fatalf("NewEvidenceReader() error = %v", err)
	}

	evidence, err := reader.ReportEvidence(t.Context(), querybase.Epoch{ID: 4, ReportSetID: "report-set"})
	if err != nil {
		t.Fatalf("ReportEvidence() error = %v", err)
	}
	if evidence.ReportView.ReportSetID != "report-set" ||
		len(evidence.Entities) != 0 || knowledge.opens != 0 {
		t.Fatalf("Report evidence = %#v; Knowledge opens = %d", evidence, knowledge.opens)
	}
}

func TestNewEvidenceReaderRejectsMissingSources(t *testing.T) {
	knowledge := &knowledgeViews{}
	if _, err := NewEvidenceReader(nil, knowledge, entityMetadata{}); err == nil {
		t.Fatal("NewEvidenceReader(nil, knowledge) error = nil")
	}
	if _, err := NewEvidenceReader(&currentReports{}, nil, entityMetadata{}); err == nil {
		t.Fatal("NewEvidenceReader(reports, nil) error = nil")
	}
}

type entityMetadata struct{}

func (entityMetadata) Entity(
	context.Context,
	knowledgedomain.Reference[knowledgedomain.EntityID],
) (provenance.EntityMetadata, error) {
	return provenance.EntityMetadata{
		Evidence: []provenance.Evidence{{TextUnitID: "text-1", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
	}, nil
}
