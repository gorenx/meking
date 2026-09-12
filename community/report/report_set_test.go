package report

import (
	"testing"

	"github.com/memoria-space/meking/community"
)

const (
	fixtureCommunitySet = community.CommunitySetID("20000000-0000-4000-8000-000000000001")
	fixtureCorpora      = "30000000-0000-4000-8000-000000000001"
)

func TestNewReportSetRequiresValidReportsAndCorpusEvidence(t *testing.T) {
	input := fixtureInput(t)
	reports := generatedFixtureReports(t, input)
	available := map[string]struct{}{
		fixtureTextA: {},
		fixtureTextB: {},
		fixtureTextC: {},
	}
	set, err := NewReportSet(
		fixtureCommunitySet,
		fixtureCorpora,
		reports,
		available,
	)
	if err != nil {
		t.Fatalf("NewReportSet() error = %v", err)
	}
	if err := ValidateReportSet(set); err != nil {
		t.Fatalf("ValidateReportSet() error = %v", err)
	}
	if len(set.Reports) != len(input.Communities) {
		t.Fatalf("ReportSet Reports = %d", len(set.Reports))
	}
	for _, report := range reports {
		if set.Reports[report.CommunityID] != report.ID {
			t.Fatalf(
				"Community %q Report = %q, want %q",
				report.CommunityID,
				set.Reports[report.CommunityID],
				report.ID,
			)
		}
	}

	missingEvidence := map[string]struct{}{
		fixtureTextA: {},
		fixtureTextB: {},
	}
	if _, err := NewReportSet(
		fixtureCommunitySet,
		fixtureCorpora,
		reports,
		missingEvidence,
	); err == nil {
		t.Fatal("NewReportSet(missing Corpus evidence) error = nil")
	}
}

func generatedFixtureReports(t *testing.T, input Input) []Report {
	t.Helper()
	generator, err := NewGenerator(
		&scriptedModel{},
		codePointCounter{},
		fixtureConfig(),
	)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	reports, err := generator.Generate(t.Context(), input)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return reports
}
