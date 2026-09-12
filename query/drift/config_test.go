package drift

import (
	"reflect"
	"testing"
)

func TestPrimerConfigRequiresPositiveBounds(t *testing.T) {
	valid := DefaultPrimerConfig()
	invalid := []func(*PrimerConfig){
		func(config *PrimerConfig) { config.CommunityLevel = -1 },
		func(config *PrimerConfig) { config.Reports = 0 },
		func(config *PrimerConfig) { config.Folds = 0 },
		func(config *PrimerConfig) { config.MaxConcurrency = 0 },
		func(config *PrimerConfig) { config.MaxPromptTokens = 0 },
		func(config *PrimerConfig) { config.HyDEMaxCompletionTokens = 0 },
		func(config *PrimerConfig) { config.PrimerMaxCompletionTokens = 0 },
	}
	for _, invalidate := range invalid {
		config := valid
		invalidate(&config)
		if err := config.Validate(); err == nil {
			t.Fatalf("Validate(%+v) error = nil", config)
		}
	}
}

func TestSplitReportsUsesDeterministicContiguousFolds(t *testing.T) {
	reports := make([]SelectedReport, 5)
	for index := range reports {
		reports[index].Report.ID = string(rune('a' + index))
	}
	groups := splitReports(reports, 3)
	if len(groups) != 3 || len(groups[0]) != 2 || len(groups[1]) != 2 || len(groups[2]) != 1 ||
		groups[0][0].Report.ID != "a" || groups[1][0].Report.ID != "c" || groups[2][0].Report.ID != "e" {
		t.Fatalf("folds = %#v", groups)
	}
	if groups := splitReports(nil, 3); len(groups) != 0 {
		t.Fatalf("empty folds = %#v", groups)
	}
}

func TestTraversalConfigRequiresPositiveHardBounds(t *testing.T) {
	valid := DefaultTraversalConfig()
	fields := []string{
		"MaxDepth", "BatchSize", "FollowUpLimit", "MaxBranches", "MaxConcurrency",
		"MaxCompletionTokens",
	}
	limitFields := []string{"ModelCalls", "PromptTokens", "OutputTokens", "Duration"}
	for _, field := range limitFields {
		config := valid
		value := reflect.ValueOf(&config.Limits).Elem().FieldByName(field)
		value.SetInt(0)
		if err := config.Validate(); err == nil {
			t.Fatalf("Validate() accepted zero request %s", field)
		}
	}
	for _, field := range fields {
		config := valid
		value := reflect.ValueOf(&config).Elem().FieldByName(field)
		value.SetInt(0)
		if err := config.Validate(); err == nil {
			t.Fatalf("Validate() accepted zero %s", field)
		}
	}
	for _, responseType := range []string{"", " ", " paragraphs "} {
		config := valid
		config.ResponseType = responseType
		if err := config.Validate(); err == nil {
			t.Fatalf("Validate() accepted response type %q", responseType)
		}
	}
}

func TestReduceConfigRequiresPositiveBounds(t *testing.T) {
	valid := DefaultReduceConfig()
	for _, config := range []ReduceConfig{
		{MaxCompletionTokens: valid.MaxCompletionTokens, ResponseType: valid.ResponseType},
		{MaxContextTokens: valid.MaxContextTokens, ResponseType: valid.ResponseType},
		{MaxContextTokens: valid.MaxContextTokens, MaxCompletionTokens: valid.MaxCompletionTokens},
	} {
		if err := config.Validate(); err == nil {
			t.Fatalf("Validate(%+v) error = nil", config)
		}
	}
}
