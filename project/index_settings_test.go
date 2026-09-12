package project

import (
	"math"
	"strings"
	"testing"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/semantic"
)

func TestDefaultIndexSettingsDescribeStandardPublication(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	if settings.Version != CurrentSettingsVersion || CurrentSettingsVersion != 5 {
		t.Fatalf("settings version = %d", settings.Version)
	}
	if settings.Index.Standard.CommunityReportPrompt != "prompts/community_report_graph.txt" {
		t.Fatalf("Standard report prompt = %q", settings.Index.Standard.CommunityReportPrompt)
	}
	if !settings.Index.ReportVectors.Enabled ||
		settings.Index.ReportVectors.BatchSize != semantic.DefaultBatchSize ||
		settings.Index.ReportVectors.BatchMaxTokens != semantic.DefaultBatchMaxTokens {
		t.Fatalf("Report vector settings = %#v", settings.Index.ReportVectors)
	}
	if settings.Index.Community.RelationChangeThreshold != community.DefaultRelationChangeThreshold ||
		community.DefaultRelationChangeThreshold != 5 {
		t.Fatalf(
			"Relation change threshold = %d, want 5",
			settings.Index.Community.RelationChangeThreshold,
		)
	}
}

func TestIndexSettingsRoundTripUsesStandardYAML(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.Index.ReportVectors.Enabled = false
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	text := string(data)
	for _, section := range []string{"    community:\n", "    report_vectors:\n", "    standard:\n"} {
		if !strings.Contains(text, section) {
			t.Fatalf("settings YAML is missing %q:\n%s", section, text)
		}
	}
	for _, obsolete := range []string{"    method:", "    fast:", "    embeddings:", "snapshot", "current_file:"} {
		if strings.Contains(text, obsolete) {
			t.Fatalf("settings YAML contains obsolete field %q:\n%s", obsolete, text)
		}
	}
	parsed, err := ParseSettings(data)
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if parsed.Index.ReportVectors.Enabled {
		t.Fatalf("parsed Report vector settings = %#v", parsed.Index.ReportVectors)
	}
}

func TestParseSettingsDefaultsOmittedReportVectorFields(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	text := string(data)
	start := strings.Index(text, "    report_vectors:\n")
	end := strings.Index(text[start:], "    standard:\n")
	if start < 0 || end < 0 {
		t.Fatalf("cannot locate Report vector section:\n%s", text)
	}
	end += start
	parsed, err := ParseSettings([]byte(text[:start] + text[end:]))
	if err != nil {
		t.Fatalf("ParseSettings(missing Report vector section) error = %v", err)
	}
	if !parsed.Index.ReportVectors.Enabled || parsed.Index.ReportVectors.BatchSize != semantic.DefaultBatchSize {
		t.Fatalf("defaulted Report vector settings = %#v", parsed.Index.ReportVectors)
	}
}

func TestParseSettingsDefaultsOmittedRelationChangeThreshold(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	withoutThreshold := strings.Replace(
		string(data),
		"        relation_change_threshold: 5\n",
		"",
		1,
	)
	if withoutThreshold == string(data) {
		t.Fatalf("generated settings omit relation_change_threshold:\n%s", data)
	}
	parsed, err := ParseSettings([]byte(withoutThreshold))
	if err != nil {
		t.Fatalf("ParseSettings(missing threshold) error = %v", err)
	}
	if parsed.Index.Community.RelationChangeThreshold != community.DefaultRelationChangeThreshold {
		t.Fatalf(
			"defaulted Relation change threshold = %d, want %d",
			parsed.Index.Community.RelationChangeThreshold,
			community.DefaultRelationChangeThreshold,
		)
	}
}

func TestSettingsRejectInvalidReportVectorPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Settings)
		want   string
	}{
		{name: "batch size", mutate: func(settings *Settings) { settings.Index.ReportVectors.BatchSize = 0 }, want: "batch limits"},
		{name: "batch tokens", mutate: func(settings *Settings) { settings.Index.ReportVectors.BatchMaxTokens = 0 }, want: "batch limits"},
		{name: "relation threshold", mutate: func(settings *Settings) { settings.Index.Community.RelationChangeThreshold = 0 }, want: "relation change threshold"},
		{name: "model", mutate: func(settings *Settings) { settings.Index.EmbeddingModelID = "missing" }, want: "unknown embedding model"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings, err := DefaultSettings("gpt-test", "embedding-test")
			if err != nil {
				t.Fatalf("DefaultSettings() error = %v", err)
			}
			test.mutate(&settings)
			if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestTextUnitVectorsRequireSelectedEmbeddingModel(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.Index.ReportVectors.Enabled = false
	settings.Index.EmbeddingModelID = ""
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "embedding model id") {
		t.Fatalf("Validate() error = %v, want required embedding model", err)
	}
}

func TestProjectIndexSettingsRejectCommunitySeedOutsideDomainRange(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.Index.Community.Seed = uint64(math.MaxInt64) + 1
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "community seed") {
		t.Fatalf("Validate() error = %v, want community seed error", err)
	}
}

func TestParseSettingsRejectsHistoricalSchema(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	historical := strings.Replace(string(data), "version: 5\n", "version: 4\n", 1)
	_, err = ParseSettings([]byte(historical))
	if err == nil || !strings.Contains(err.Error(), "unsupported settings version 4") {
		t.Fatalf("ParseSettings(historical) error = %v", err)
	}
}
