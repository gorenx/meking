package report

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memoria-space/meking/community"
	projectprompts "github.com/memoria-space/meking/project/prompts"
)

type codePointCounter struct{}

func (codePointCounter) Count(text string) (int, error) {
	return len([]rune(text)), nil
}

type scriptedResult struct {
	draft community.ReportDraft
	err   error
}

type scriptedModel struct {
	prompts         []string
	results         []scriptedResult
	fragmentPrompts []string
	fragments       []community.ReportFragment
	fragmentError   error
}

func (m *scriptedModel) GenerateReportFragment(
	_ context.Context,
	request community.ReportFragmentRequest,
) (community.ReportFragment, error) {
	m.fragmentPrompts = append(m.fragmentPrompts, request.Prompt)
	if m.fragmentError != nil {
		return community.ReportFragment{}, m.fragmentError
	}
	index := len(m.fragmentPrompts) - 1
	if index < len(m.fragments) {
		return m.fragments[index], nil
	}
	return community.ReportFragment{Content: "fragment"}, nil
}

func (m *scriptedModel) GenerateCommunityReport(
	_ context.Context,
	request community.ReportModelRequest,
) (community.ReportDraft, error) {
	index := len(m.prompts)
	m.prompts = append(m.prompts, request.Prompt)
	if index < len(m.results) {
		return m.results[index].draft, m.results[index].err
	}
	return fixtureDraft(index + 1), nil
}

func TestGeneratorCreatesCompleteImmutableReports(t *testing.T) {
	model := &scriptedModel{}
	generator, err := NewGenerator(model, codePointCounter{}, fixtureConfig())
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	if generator.Settings() != settingsFromConfig(fixtureConfig()) {
		t.Fatalf("Generator Settings = %#v", generator.Settings())
	}
	input := fixtureInput(t)
	reports, err := generator.Generate(t.Context(), input)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(reports) != 3 || len(model.prompts) != 3 {
		t.Fatalf("reports/prompts = %d/%d, want 3/3", len(reports), len(model.prompts))
	}
	byID := make(map[community.CommunityID]community.Membership)
	for _, current := range input.Communities {
		byID[current.ID] = current
	}
	gotNumbers := make([]int, len(reports))
	for index, report := range reports {
		if err := ValidateReport(report); err != nil {
			t.Fatalf("ValidateReport(%d) error = %v", index, err)
		}
		gotNumbers[index] = byID[report.CommunityID].Number
		if report.Settings.Model != "report-model" ||
			report.Settings.Tokenizer != "o200k_base" {
			t.Fatalf("Report settings = %#v", report.Settings)
		}
	}
	if !reflect.DeepEqual(gotNumbers, []int{1, 2, 0}) {
		t.Fatalf("Community order = %v, want [1 2 0]", gotNumbers)
	}
	first := reports[0]
	if got := []string{
		first.EntitySources[0].ID,
		first.EntitySources[1].ID,
	}; !reflect.DeepEqual(got, []string{fixtureEntityB, fixtureEntityA}) {
		t.Fatalf("Entity source order = %v", got)
	}
	if len(first.RelationSources) != 1 ||
		first.RelationSources[0].ID != fixtureRelationAB ||
		len(first.ClaimSources) != 2 {
		t.Fatalf(
			"Report sources = %#v / %#v",
			first.RelationSources,
			first.ClaimSources,
		)
	}
	if !strings.Contains(model.prompts[0], "-----Entities-----") ||
		!strings.Contains(model.prompts[0], "0,B,Entity B,3") ||
		!strings.Contains(model.prompts[0], "A subject,A object,FACT") ||
		!strings.Contains(model.prompts[0], "A,B,A to B,1.5,5") {
		t.Fatalf("complete prompt = %q", model.prompts[0])
	}
}

func TestGeneratorRejectsWholeCollectionWhenOneModelCallFails(t *testing.T) {
	model := &scriptedModel{results: []scriptedResult{{
		err: errors.New("provider failed"),
	}}}
	generator, err := NewGenerator(model, codePointCounter{}, fixtureConfig())
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	reports, err := generator.Generate(t.Context(), fixtureInput(t))
	if err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("Generate() error = %v", err)
	}
	if reports != nil {
		t.Fatalf("Generate() reports = %#v, want nil", reports)
	}
}

func TestGeneratorRejectsCompletePromptOverBudget(t *testing.T) {
	model := &scriptedModel{}
	config := fixtureConfig()
	config.MaxInputTokens = 1
	generator, err := NewGenerator(model, codePointCounter{}, config)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	_, err = generator.Generate(t.Context(), fixtureInput(t))
	if !errors.Is(err, ErrReportInputTooLarge) {
		t.Fatalf("Generate() error = %v, want ErrReportInputTooLarge", err)
	}
	if len(model.prompts) != 0 {
		t.Fatalf("model calls = %d, want zero", len(model.prompts))
	}
}

type fragmentRoutingCounter struct{}

func (fragmentRoutingCounter) Count(text string) (int, error) {
	if strings.HasPrefix(text, "Context:") {
		if strings.Contains(text, "-----Fragment 0-----") {
			return 10, nil
		}
		entities := 0
		for _, description := range []string{"Entity A", "Entity B", "Entity C", "Entity D"} {
			if strings.Contains(text, description) {
				entities++
			}
		}
		if entities > 2 {
			return 100, nil
		}
		return 10, nil
	}
	if strings.HasPrefix(text, "You summarize one evidence batch") {
		entities := 0
		for _, description := range []string{"Entity A", "Entity B", "Entity C", "Entity D"} {
			if strings.Contains(text, description) {
				entities++
			}
		}
		if entities > 2 {
			return 100, nil
		}
		return 10, nil
	}
	return 10, nil
}

func TestGeneratorFragmentsOverBudgetEvidenceWithoutChangingSourceRows(t *testing.T) {
	input := fixtureInput(t)

	config := fixtureConfig()
	config.MaxInputTokens = 50
	model := &scriptedModel{}
	generator, err := NewGenerator(model, fragmentRoutingCounter{}, config)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	reports, err := generator.Generate(t.Context(), input)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(reports) != 3 || len(model.fragmentPrompts) != 2 || len(model.prompts) != 3 {
		t.Fatalf(
			"reports/fragment calls/final calls = %d/%d/%d, want 3/2/3",
			len(reports), len(model.fragmentPrompts), len(model.prompts),
		)
	}
	if !strings.Contains(model.fragmentPrompts[0], "0,B,Entity B,3") ||
		!strings.Contains(model.fragmentPrompts[0], "2,A,Entity A,2") ||
		!strings.Contains(model.fragmentPrompts[0], "1,A,B,A to B,1.5,5") {
		t.Fatalf("first Fragment lost complete-input row identities: %q", model.fragmentPrompts[0])
	}
	if !strings.Contains(model.fragmentPrompts[1], "0,B,C,B to C,1,6") ||
		!strings.Contains(model.fragmentPrompts[1], "2,C,D,C to D,2,5") {
		t.Fatalf("second Fragment lost cross-region Relation evidence: %q", model.fragmentPrompts[1])
	}
	rootReport := reports[2]
	if len(rootReport.EntitySources) != 4 || len(rootReport.RelationSources) != 3 ||
		len(rootReport.ClaimSources) != 2 {
		t.Fatalf("Report sources = %#v", rootReport)
	}
	if !strings.Contains(model.prompts[2], "-----Fragment 0-----") ||
		!strings.Contains(model.prompts[2], "-----Fragment 1-----") {
		t.Fatalf("final merge prompt = %q", model.prompts[2])
	}
}

func TestGeneratorRejectsCollectionWhenRequiredFragmentFails(t *testing.T) {
	input := fixtureInput(t)
	root := input.Communities[0]
	root.Final = true
	input.Communities = []community.Membership{root}
	config := fixtureConfig()
	config.MaxInputTokens = 50
	model := &scriptedModel{fragmentError: errors.New("fragment unavailable")}
	generator, err := NewGenerator(model, fragmentRoutingCounter{}, config)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	reports, err := generator.Generate(t.Context(), input)
	if err == nil || !strings.Contains(err.Error(), "fragment unavailable") {
		t.Fatalf("Generate() error = %v", err)
	}
	if reports != nil || len(model.prompts) != 0 {
		t.Fatalf("partial Reports/final calls = %#v/%d", reports, len(model.prompts))
	}
}

func TestGeneratorTokenPacksUnsplittableCommunity(t *testing.T) {
	input := fixtureInput(t)
	root := input.Communities[0]
	root.Final = true
	root.Unsplittable = true
	input.Communities = []community.Membership{root}
	config := fixtureConfig()
	config.MaxInputTokens = 50
	model := &scriptedModel{}
	generator, err := NewGenerator(model, fragmentRoutingCounter{}, config)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	reports, err := generator.Generate(t.Context(), input)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(reports) != 1 || len(model.fragmentPrompts) < 2 || len(model.prompts) != 1 {
		t.Fatalf(
			"reports/fragment calls/final calls = %d/%d/%d",
			len(reports), len(model.fragmentPrompts), len(model.prompts),
		)
	}
}

type fragmentMarkerCounter struct{}

func (fragmentMarkerCounter) Count(text string) (int, error) {
	return strings.Count(text, "-----Fragment ") * 10, nil
}

func TestWidestFragmentGroupsUsesMaximumSafeWidth(t *testing.T) {
	generator := &Generator{
		tokens: fragmentMarkerCounter{},
		config: Config{MaxInputTokens: 35, MaxReportLength: 100},
	}
	groups, reduced, err := generator.widestFragmentGroups([]string{"a", "b", "c", "d", "e"})
	if err != nil {
		t.Fatalf("widestFragmentGroups() error = %v", err)
	}
	if !reduced || len(groups) != 2 || len(groups[0]) != 3 || len(groups[1]) != 2 {
		t.Fatalf("groups/reduced = %#v/%v, want widths 3 and 2", groups, reduced)
	}
}

func TestNewGeneratorRejectsNegativeConcurrency(t *testing.T) {
	config := fixtureConfig()
	config.MaxConcurrency = -1
	if _, err := NewGenerator(
		&scriptedModel{},
		codePointCounter{},
		config,
	); err == nil {
		t.Fatal("NewGenerator() accepted negative concurrency")
	}
}

func TestGeneratorRendersDefaultProjectPrompt(t *testing.T) {
	model := &scriptedModel{}
	config := fixtureConfig()
	config.Prompt = projectprompts.CommunityReportGraph
	generator, err := NewGenerator(model, codePointCounter{}, config)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	if _, err := generator.Generate(t.Context(), fixtureInput(t)); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(model.prompts) != 3 ||
		!strings.Contains(model.prompts[0], "# Real Data") ||
		!strings.Contains(model.prompts[0], `"findings": [`) {
		t.Fatalf("default Report prompt = %q", model.prompts[0])
	}
}

func TestGeneratorBoundsSameLevelConcurrencyAndKeepsLevelsSequential(t *testing.T) {
	model := &boundedModel{
		started: make(chan struct{}, 3),
		release: make(chan struct{}),
	}
	config := fixtureConfig()
	config.MaxConcurrency = 2
	generator, err := NewGenerator(model, codePointCounter{}, config)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	results := make(chan []Report, 1)
	failures := make(chan error, 1)
	go func() {
		reports, runErr := generator.Generate(t.Context(), fixtureInput(t))
		results <- reports
		failures <- runErr
	}()

	waitForStarts(t, model.started, 2)
	select {
	case <-model.started:
		t.Fatal("parent level started before the child level completed")
	case <-time.After(30 * time.Millisecond):
	}
	close(model.release)
	if err := <-failures; err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if reports := <-results; len(reports) != 3 {
		t.Fatalf("reports = %d, want 3", len(reports))
	}
	if model.maximum.Load() != 2 {
		t.Fatalf("maximum concurrent calls = %d, want 2", model.maximum.Load())
	}
}

type boundedModel struct {
	started chan struct{}
	release chan struct{}
	current atomic.Int64
	maximum atomic.Int64
}

func (m *boundedModel) GenerateCommunityReport(
	context.Context,
	community.ReportModelRequest,
) (community.ReportDraft, error) {
	current := m.current.Add(1)
	for maximum := m.maximum.Load(); current > maximum &&
		!m.maximum.CompareAndSwap(maximum, current); maximum = m.maximum.Load() {
	}
	m.started <- struct{}{}
	<-m.release
	m.current.Add(-1)
	return fixtureDraft(1), nil
}

func (m *boundedModel) GenerateReportFragment(
	ctx context.Context,
	_ community.ReportFragmentRequest,
) (community.ReportFragment, error) {
	_, err := m.GenerateCommunityReport(ctx, community.ReportModelRequest{})
	return community.ReportFragment{Content: "fragment"}, err
}

func waitForStarts(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for concurrent Report %d", index+1)
		}
	}
}

func fixtureDraft(sequence int) community.ReportDraft {
	number := strconv.Itoa(sequence)
	return community.ReportDraft{
		Title:   "Report " + number,
		Summary: "Summary " + number,
		Findings: []community.ReportFinding{{
			Summary:     "Finding " + number,
			Explanation: "Explanation " + number,
		}},
		Rating:            float64(sequence) + 0.5,
		RatingExplanation: "Rating explanation " + number,
	}
}

const (
	fixtureEntityA    = "00000000-0000-4000-8000-000000000001"
	fixtureEntityB    = "00000000-0000-4000-8000-000000000002"
	fixtureEntityC    = "00000000-0000-4000-8000-000000000003"
	fixtureEntityD    = "00000000-0000-4000-8000-000000000004"
	fixtureRelationAB = "10000000-0000-4000-8000-000000000001"
	fixtureRelationBC = "10000000-0000-4000-8000-000000000002"
	fixtureRelationCD = "10000000-0000-4000-8000-000000000003"
	fixtureClaimA     = "20000000-0000-4000-8000-000000000001"
	fixtureClaimAB    = "20000000-0000-4000-8000-000000000002"
	fixtureTextA      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixtureTextB      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	fixtureTextC      = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func fixtureConfig() Config {
	return Config{
		Prompt: "Context:\n{input_text}\nMax:{max_report_length}",
		Model:  "report-model", Tokenizer: "o200k_base",
		MaxInputTokens: 10_000, MaxReportLength: 2_000, MaxConcurrency: 1,
	}
}

func fixtureInput(t *testing.T) Input {
	t.Helper()
	hierarchy := community.Hierarchy{Communities: []community.Community{
		{
			ID: 0, Level: 0, ParentID: -1,
			Nodes: []string{fixtureEntityA, fixtureEntityB, fixtureEntityC, fixtureEntityD},
		},
		{
			ID: 1, Level: 1, ParentID: 0,
			Nodes: []string{fixtureEntityA, fixtureEntityB}, Final: true,
		},
		{
			ID: 2, Level: 1, ParentID: 0,
			Nodes: []string{fixtureEntityC, fixtureEntityD}, Final: true,
		},
	}}
	set, err := community.NewCommunitySet(
		hierarchy,
		[]community.EntityReference{
			{ID: fixtureEntityA, Version: 1},
			{ID: fixtureEntityB, Version: 1},
			{ID: fixtureEntityC, Version: 1},
			{ID: fixtureEntityD, Version: 1},
		},
		[]community.RelationReference{
			{ID: fixtureRelationAB, Version: 1},
			{ID: fixtureRelationBC, Version: 1},
			{ID: fixtureRelationCD, Version: 1},
		},
		community.DefaultDetectConfig(),
	)
	if err != nil {
		t.Fatalf("NewCommunitySet() error = %v", err)
	}
	return Input{
		Communities: set.Communities,
		Entities: []Entity{
			{
				ID: fixtureEntityA, Version: 1, Title: "A",
				Description: "Entity A", Degree: 2, TextUnitIDs: []string{fixtureTextA},
			},
			{
				ID: fixtureEntityB, Version: 1, Title: "B",
				Description: "Entity B", Degree: 3, TextUnitIDs: []string{fixtureTextB},
			},
			{
				ID: fixtureEntityC, Version: 1, Title: "C",
				Description: "Entity C", Degree: 3, TextUnitIDs: []string{fixtureTextB},
			},
			{
				ID: fixtureEntityD, Version: 1, Title: "D",
				Description: "Entity D", Degree: 2, TextUnitIDs: []string{fixtureTextC},
			},
		},
		Relations: []Relation{
			{
				ID: fixtureRelationAB, Version: 1,
				SourceEntityID: fixtureEntityA, TargetEntityID: fixtureEntityB,
				Description: "A to B", Weight: 1.5, CombinedDegree: 5,
				TextUnitIDs: []string{fixtureTextA},
			},
			{
				ID: fixtureRelationBC, Version: 1,
				SourceEntityID: fixtureEntityB, TargetEntityID: fixtureEntityC,
				Description: "B to C", Weight: 1, CombinedDegree: 6,
				TextUnitIDs: []string{fixtureTextB},
			},
			{
				ID: fixtureRelationCD, Version: 1,
				SourceEntityID: fixtureEntityC, TargetEntityID: fixtureEntityD,
				Description: "C to D", Weight: 2, CombinedDegree: 5,
				TextUnitIDs: []string{fixtureTextC},
			},
		},
		Claims: []Claim{
			{
				ID: fixtureClaimA, Version: 1, EvidenceIndex: 0,
				Subject:     ClaimSubject{Kind: EntityClaimSubject, ID: fixtureEntityA},
				SubjectText: "A subject", ObjectText: "A object", Type: "FACT",
				Status: "TRUE", Description: "A claim", SourceText: "A source",
				TextUnitID: fixtureTextA,
			},
			{
				ID: fixtureClaimAB, Version: 1, EvidenceIndex: 0,
				Subject:     ClaimSubject{Kind: RelationClaimSubject, ID: fixtureRelationAB},
				SubjectText: "A B subject", ObjectText: "linked", Type: "LINK",
				Status: "TRUE", Description: "A B claim", SourceText: "A B source",
				TextUnitID: fixtureTextA,
			},
		},
		TextUnitIDs: []string{fixtureTextA, fixtureTextB, fixtureTextC},
		Period:      "2026-07-24",
	}
}
