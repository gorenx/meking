package global

import (
	"context"
	"reflect"
	"strings"
	"testing"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

type codePointCounter struct{}

func (codePointCounter) Count(_ context.Context, _ string, text string) (int, error) {
	return len([]rune(text)), nil
}

type selectorStub struct {
	request SelectionRequest
	ids     []string
	err     error
}

func (s *selectorStub) SelectCommunities(
	_ context.Context,
	request SelectionRequest,
) ([]string, error) {
	s.request = request
	return append([]string(nil), s.ids...), s.err
}

func TestContextUsesEveryEligibleCommunityOnceWithoutTitleDeduplication(t *testing.T) {
	builder := newTestContextBuilder(t)
	config := DefaultContextConfig()
	config.ShuffleData = false
	config.IncludeCommunityWeight = false
	evidence := testReportEvidence(
		[]queryreport.PublishedCommunity{
			{ID: "community-a", Number: 0, Level: 0, EntityIDs: []string{"entity-a"}},
			{ID: "community-b", Number: 1, Level: 0, EntityIDs: []string{"entity-b"}},
		},
		[]queryreport.PublishedReport{
			testReport("report-a", "community-a", "Shared title", "entity-a", 1),
			testReport("report-b", "community-b", "Shared title", "entity-b", 1),
		},
		nil,
	)

	result, err := builder.Build(t.Context(), ContextRequest{Evidence: evidence, Config: config})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Reports) != 2 || result.Reports[0].CommunityID != "community-a" ||
		result.Reports[1].CommunityID != "community-b" {
		t.Fatalf("Reports = %#v, want both distinct Communities", result.Reports)
	}
}

func TestContextWeightsTheExactEntityVersionsUsedByEachReport(t *testing.T) {
	builder := newTestContextBuilder(t)
	config := DefaultContextConfig()
	config.ShuffleData = false
	config.IncludeCommunityWeight = true
	config.NormalizeCommunityWeight = false
	evidence := testReportEvidence(
		[]queryreport.PublishedCommunity{
			{ID: "community-old", Number: 0, Level: 0, EntityIDs: []string{"entity-a"}},
			{ID: "community-new", Number: 1, Level: 0, EntityIDs: []string{"entity-a"}},
		},
		[]queryreport.PublishedReport{
			testReport("report-old", "community-old", "Old", "entity-a", 1),
			testReport("report-new", "community-new", "New", "entity-a", 2),
		},
		[]EntityEvidence{
			{Reference: querybase.KnowledgeReference{ID: "entity-a", Version: 1}, TextUnitIDs: []string{"unit-a"}},
			{Reference: querybase.KnowledgeReference{ID: "entity-a", Version: 2}, TextUnitIDs: []string{"unit-a", "unit-b"}},
		},
	)

	result, err := builder.Build(t.Context(), ContextRequest{Evidence: evidence, Config: config})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Chunks) != 1 {
		t.Fatalf("Chunks = %#v, want one", result.Chunks)
	}
	rows := result.Chunks[0].Rows
	if len(rows) != 2 || rows[0].Values[0] != "1" || rows[0].Values[2] != "2.0" ||
		rows[1].Values[0] != "0" || rows[1].Values[2] != "1.0" {
		t.Fatalf("weighted rows = %#v", rows)
	}
	if !reflect.DeepEqual(result.Chunks[0].ReportIDs, []int{1, 0}) {
		t.Fatalf("Report IDs = %v, want request-local IDs [1 0]", result.Chunks[0].ReportIDs)
	}
}

func TestContextPreservesDynamicSelectionOrderBeforeBatchOrdering(t *testing.T) {
	selector := &selectorStub{ids: []string{"community-b", "community-a"}}
	builder, err := NewContextBuilder(codePointCounter{}, selector)
	if err != nil {
		t.Fatalf("NewContextBuilder() error = %v", err)
	}
	config := DefaultContextConfig()
	config.DynamicSelection = true
	config.ShuffleData = false
	config.IncludeCommunityWeight = false
	evidence := testReportEvidence(
		[]queryreport.PublishedCommunity{
			{ID: "community-a", Number: 0, Level: 0, EntityIDs: []string{"entity-a"}},
			{ID: "community-b", Number: 1, Level: 0, EntityIDs: []string{"entity-b"}},
			{ID: "community-hidden", Number: 2, Level: 3, EntityIDs: []string{"entity-c"}},
		},
		[]queryreport.PublishedReport{
			testReport("report-a", "community-a", "A", "entity-a", 1),
			testReport("report-b", "community-b", "B", "entity-b", 1),
			testReport("report-hidden", "community-hidden", "Hidden", "entity-c", 1),
		},
		nil,
	)

	result, err := builder.Build(t.Context(), ContextRequest{
		Evidence: evidence, Question: "question", Config: config,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := candidateCommunityIDs(selector.request.Reports); !reflect.DeepEqual(
		got,
		[]string{"community-a", "community-b"},
	) {
		t.Fatalf("selector candidates = %v", got)
	}
	if len(result.Reports) != 2 || result.Reports[0].CommunityID != "community-b" ||
		result.Reports[0].RecordID != 0 || result.Reports[1].CommunityID != "community-a" ||
		result.Reports[1].RecordID != 1 {
		t.Fatalf("selected Reports = %#v", result.Reports)
	}
}

func TestContextAppliesMinimumRankAfterDynamicSelection(t *testing.T) {
	selector := &selectorStub{ids: []string{"community-root", "community-child"}}
	builder, err := NewContextBuilder(codePointCounter{}, selector)
	if err != nil {
		t.Fatalf("NewContextBuilder() error = %v", err)
	}
	config := DefaultContextConfig()
	config.DynamicSelection = true
	config.ShuffleData = false
	config.IncludeCommunityWeight = false
	config.MinimumCommunityRank = 0.5
	rootReport := testReport("report-root", "community-root", "Root", "entity-a", 1)
	rootReport.Rank = 0.2
	childReport := testReport("report-child", "community-child", "Child", "entity-b", 1)
	childReport.Rank = 0.8
	evidence := testReportEvidence(
		[]queryreport.PublishedCommunity{
			{ID: "community-root", Number: 0, Level: 0, EntityIDs: []string{"entity-a"}},
			{ID: "community-child", Number: 1, Level: 1, ParentID: stringPointer("community-root"), EntityIDs: []string{"entity-b"}},
		},
		[]queryreport.PublishedReport{rootReport, childReport},
		nil,
	)

	result, err := builder.Build(t.Context(), ContextRequest{Evidence: evidence, Config: config})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := candidateCommunityIDs(selector.request.Reports); !reflect.DeepEqual(
		got,
		[]string{"community-root", "community-child"},
	) {
		t.Fatalf("selector candidates = %v, want both hierarchy Reports", got)
	}
	if len(result.Reports) != 1 || result.Reports[0].CommunityID != "community-child" ||
		result.Reports[0].RecordID != 0 {
		t.Fatalf("rank-filtered Reports = %#v", result.Reports)
	}
}

func TestContextIncludesAnOversizedFirstReport(t *testing.T) {
	builder := newTestContextBuilder(t)
	config := DefaultContextConfig()
	config.MaxContextTokens = 1
	config.ShuffleData = false
	config.IncludeCommunityWeight = false
	report := testReport("report-a", "community-a", "A", "entity-a", 1)
	report.FullContent = strings.Repeat("evidence ", 20)
	evidence := testReportEvidence(
		[]queryreport.PublishedCommunity{{
			ID: "community-a", Number: 0, Level: 0, EntityIDs: []string{"entity-a"},
		}},
		[]queryreport.PublishedReport{report},
		nil,
	)

	result, err := builder.Build(t.Context(), ContextRequest{Evidence: evidence, Config: config})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Chunks) != 1 || !strings.Contains(result.Chunks[0].Text, "evidence") {
		t.Fatalf("Context = %#v, want the first oversized Report", result)
	}
}

func TestContextCarriesFixedPublicationAndConversationEvidence(t *testing.T) {
	builder := newTestContextBuilder(t)
	config := DefaultContextConfig()
	config.ShuffleData = false
	config.IncludeCommunityWeight = false
	config.ConversationUserTurnsOnly = false
	evidence := testReportEvidence(
		[]queryreport.PublishedCommunity{{
			ID: "community-a", Number: 0, Level: 0, EntityIDs: []string{"entity-a"},
		}},
		[]queryreport.PublishedReport{
			testReport("report-a", "community-a", "A", "entity-a", 1),
		},
		nil,
	)
	result, err := builder.Build(t.Context(), ContextRequest{
		Evidence: evidence,
		Conversation: []querybase.ConversationTurn{
			{Role: querybase.RoleUser, Content: "Earlier question"},
			{Role: querybase.RoleAssistant, Content: "Earlier answer"},
		},
		Config: config,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.ReportSetID != "report-set" || result.CommunitySetID != "community-set" ||
		result.CorporaID != "corpora" || len(result.Sections) != 2 ||
		!strings.Contains(result.Chunks[0].Text, "Earlier answer") {
		t.Fatalf("Context = %#v", result)
	}
}

func TestContextReturnsNoChunksWithoutEligibleReports(t *testing.T) {
	builder := newTestContextBuilder(t)
	result, err := builder.Build(t.Context(), ContextRequest{
		Evidence: testReportEvidence(nil, nil, nil), Config: DefaultContextConfig(),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Chunks) != 0 || len(result.Reports) != 0 || len(result.Sections) != 0 {
		t.Fatalf("Context = %#v, want empty evidence", result)
	}
}

func TestContextRejectsInvalidRequestParameters(t *testing.T) {
	builder := newTestContextBuilder(t)
	config := DefaultContextConfig()
	config.MaxContextTokens = 0
	if _, err := builder.Build(t.Context(), ContextRequest{Config: config}); err == nil {
		t.Fatal("Build() with zero token budget error = nil")
	}

	config = DefaultContextConfig()
	_, err := builder.Build(t.Context(), ContextRequest{
		Config: config,
		Conversation: []querybase.ConversationTurn{{
			Role: "tool", Content: "unsupported request role",
		}},
	})
	if err == nil {
		t.Fatal("Build() with unsupported conversation role error = nil")
	}
}

func newTestContextBuilder(t *testing.T) *ContextBuilder {
	t.Helper()
	builder, err := NewContextBuilder(codePointCounter{}, nil)
	if err != nil {
		t.Fatalf("NewContextBuilder() error = %v", err)
	}
	return builder
}

func testReportEvidence(
	communities []queryreport.PublishedCommunity,
	reports []queryreport.PublishedReport,
	entities []EntityEvidence,
) ReportEvidence {
	return ReportEvidence{
		ReportView: queryreport.View{
			ReportSetID: "report-set", CommunitySetID: "community-set", CorporaID: "corpora",
			Communities: communities, Reports: reports,
		},
		Entities: entities,
	}
}

func testReport(
	id string,
	communityID string,
	title string,
	entityID string,
	version uint64,
) queryreport.PublishedReport {
	return queryreport.PublishedReport{
		ID: id, CommunityID: communityID, Title: title, Summary: title + " summary",
		FullContent: title + " content", Rank: 1,
		Sources: queryreport.ReportSources{Entities: []querybase.KnowledgeReference{{
			ID: entityID, Version: version,
		}}},
	}
}

func candidateCommunityIDs(values []ReportCandidate) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.Community.ID
	}
	return result
}
