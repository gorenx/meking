package adapter

import (
	"reflect"
	"testing"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	querybase "github.com/memoria-space/meking/query"
)

func TestReportProjectionPreservesSourceOrderAndIsolatesSlices(t *testing.T) {
	source := communityreport.Report{
		ID:          "report-1",
		CommunityID: "community-1",
		Findings: []community.ReportFinding{
			{Summary: "finding", Explanation: "evidence"},
		},
		EntitySources: []community.EntityReference{
			{ID: "entity-b", Version: 2},
			{ID: "entity-a", Version: 1},
		},
		RelationSources: []community.RelationReference{
			{ID: "relation-b", Version: 4},
			{ID: "relation-a", Version: 3},
		},
		ClaimSources: []communityreport.ClaimSource{
			{ID: "claim-b", Version: 6, EvidenceIndex: 1},
			{ID: "claim-a", Version: 5, EvidenceIndex: 0},
		},
		TextUnitIDs: []string{"unit-a", "unit-b"},
	}

	got := reportProjection(source)
	wantEntities := []querybase.KnowledgeReference{
		{ID: "entity-b", Version: 2},
		{ID: "entity-a", Version: 1},
	}
	wantRelations := []querybase.KnowledgeReference{
		{ID: "relation-b", Version: 4},
		{ID: "relation-a", Version: 3},
	}
	wantClaims := []querybase.ClaimReference{
		{ID: "claim-b", Version: 6, EvidenceIndex: 1},
		{ID: "claim-a", Version: 5, EvidenceIndex: 0},
	}
	if !reflect.DeepEqual(got.Sources.Entities, wantEntities) {
		t.Fatalf("Entity sources = %#v, want %#v", got.Sources.Entities, wantEntities)
	}
	if !reflect.DeepEqual(got.Sources.Relations, wantRelations) {
		t.Fatalf("Relation sources = %#v, want %#v", got.Sources.Relations, wantRelations)
	}
	if !reflect.DeepEqual(got.Sources.Claims, wantClaims) {
		t.Fatalf("Claim sources = %#v, want %#v", got.Sources.Claims, wantClaims)
	}

	source.Findings[0].Summary = "changed"
	source.TextUnitIDs[0] = "changed"
	if got.Findings[0].Summary != "finding" || got.Sources.TextUnitIDs[0] != "unit-a" {
		t.Fatalf("projection changed with provider slices: %#v", got)
	}
}
