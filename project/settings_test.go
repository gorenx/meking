package project

import (
	"strings"
	"testing"

	"github.com/memoria-space/meking/analysis"
	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/knowledge/extraction"
	"github.com/memoria-space/meking/semantic"
)

func TestSettingsRoundTrip(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.Input.FilePattern = `.*\.md$`
	settings.Input.Encoding = "utf-8"
	completion := settings.CompletionModels[settings.Index.CompletionModelID]
	completion.BaseURL = "https://completion.example/v1"
	settings.CompletionModels[settings.Index.CompletionModelID] = completion
	embedding := settings.EmbeddingModels[settings.Index.EmbeddingModelID]
	embedding.BaseURL = "https://embedding.example/v1"
	settings.EmbeddingModels[settings.Index.EmbeddingModelID] = embedding
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	if strings.Contains(string(data), "sk-") {
		t.Fatal("generated settings contain an API key-like value")
	}
	if !strings.Contains(string(data), "${MEKING_API_KEY}") {
		t.Fatal("generated settings do not reference MEKING_API_KEY")
	}

	parsed, err := ParseSettings(data)
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if got := parsed.CompletionModels["default_completion_model"].Model; got != "gpt-test" {
		t.Fatalf("completion model = %q, want %q", got, "gpt-test")
	}
	if got := parsed.EmbeddingModels["default_embedding_model"].Model; got != "embedding-test" {
		t.Fatalf("embedding model = %q, want %q", got, "embedding-test")
	}
	if got := parsed.CompletionModels["default_completion_model"].BaseURL; got != "https://completion.example/v1" {
		t.Fatalf("completion base URL = %q", got)
	}
	if got := parsed.CompletionModels["default_completion_model"].StructuredOutput; got != StructuredOutputJSONSchema {
		t.Fatalf("completion structured output = %q", got)
	}
	if got := parsed.EmbeddingModels["default_embedding_model"].BaseURL; got != "https://embedding.example/v1" {
		t.Fatalf("embedding base URL = %q", got)
	}
	if got, want := parsed.Input.FilePattern, `.*\.md$`; got != want {
		t.Fatalf("input file pattern = %q, want %q", got, want)
	}
	if got, want := parsed.Input.Encoding, "utf-8"; got != want {
		t.Fatalf("input encoding = %q, want %q", got, want)
	}
	if parsed.Index.Community.MaxReportLength != community.DefaultReportMaxLength ||
		parsed.Index.Community.MaxReportInputTokens != community.DefaultReportMaxInputTokens ||
		parsed.Index.Community.RelationChangeThreshold != community.DefaultRelationChangeThreshold {
		t.Fatalf("community report limits = %d/%d, want defaults", parsed.Index.Community.MaxReportLength, parsed.Index.Community.MaxReportInputTokens)
	}
	if !parsed.Index.ReportVectors.Enabled ||
		parsed.Index.ReportVectors.BatchSize != semantic.DefaultBatchSize ||
		parsed.Index.ReportVectors.BatchMaxTokens != semantic.DefaultBatchMaxTokens {
		t.Fatalf("Report vector settings = %#v, want defaults", parsed.Index.ReportVectors)
	}
	if parsed.ConcurrentRequests != DefaultConcurrentRequests {
		t.Fatalf("concurrent requests = %d, want %d", parsed.ConcurrentRequests, DefaultConcurrentRequests)
	}
	if got, want := parsed.Query.Global, defaultGlobalSearchConfig(); got != want {
		t.Fatalf("global search settings = %+v, want %+v", got, want)
	}
	if got, want := parsed.Query.Drift, defaultDriftSearchConfig(); got != want {
		t.Fatalf("DRIFT search settings = %+v, want %+v", got, want)
	}
	if !parsed.Index.Community.UseLargestConnectedComponent {
		t.Fatal("use_lcc = false, want Python default true")
	}
	if parsed.Index.Standard.Claims.Enabled || parsed.Index.Standard.Claims.Prompt != "prompts/extract_claims.txt" ||
		parsed.Index.Standard.Claims.Description != extraction.DefaultClaimDescription ||
		parsed.Index.Standard.Claims.MaxGleanings != extraction.DefaultClaimMaxGleanings {
		t.Fatalf("Claim settings = %#v", parsed.Index.Standard.Claims)
	}
}

func TestDefaultSettingsExposeModelBaseURLs(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	if got := strings.Count(string(data), "base_url: \"\""); got != 2 {
		t.Fatalf("generated empty base URL fields = %d, want one for each default model:\n%s", got, data)
	}
}

func TestSettingsAcceptsSentenceChunkingWithoutTokenWindow(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.Chunking.Type = "sentence"
	settings.Chunking.Size = 0
	settings.Chunking.Overlap = 0
	if err := settings.Validate(); err != nil {
		t.Fatalf("Validate(sentence) error = %v", err)
	}
	settings.Chunking.Type = "semantic"
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported chunking type") {
		t.Fatalf("Validate(semantic) error = %v", err)
	}
}

func TestSettingsAcceptsBoundedRichDocumentInput(t *testing.T) {
	t.Parallel()
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.Input.Type = InputTypeRichDocuments
	settings.Input.FilePattern = `(?i).*\.(pdf|docx)$`
	if err := settings.Validate(); err != nil {
		t.Fatalf("Validate(default rich input) error = %v", err)
	}
	if settings.Input.RichDocumentMaxBytes() != DefaultRichDocumentMaxBytes ||
		MaximumRichDocumentMaxBytes != analysis.MaxDocumentContentBytes {
		t.Fatalf("rich document limits = %d/%d/%d", settings.Input.RichDocumentMaxBytes(), MaximumRichDocumentMaxBytes, analysis.MaxDocumentContentBytes)
	}
	settings.Input.MaxFileBytes = 1024
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	parsed, err := ParseSettings(data)
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if parsed.Input.Type != InputTypeRichDocuments || parsed.Input.MaxFileBytes != 1024 || parsed.Input.Encoding != "" {
		t.Fatalf("parsed rich input = %#v", parsed.Input)
	}

	settings.Input.Encoding = "utf-8"
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "does not use text encoding") {
		t.Fatalf("Validate(rich encoding) error = %v", err)
	}
	settings.Input.Encoding = ""
	settings.Input.MaxFileBytes = MaximumRichDocumentMaxBytes + 1
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "max file bytes") {
		t.Fatalf("Validate(oversized rich policy) error = %v", err)
	}
	settings.Input.Type = InputTypeText
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "does not use max file bytes") {
		t.Fatalf("Validate(text binary policy) error = %v", err)
	}
}

func TestSettingsRejectsInputDirectoryOutsideProject(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatal(err)
	}
	settings.Input.BaseDir = "../input"
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "input base directory") {
		t.Fatalf("Validate() error = %v, want invalid input base directory", err)
	}
}

func TestSettingsRejectsInvalidInputFilePattern(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatal(err)
	}
	settings.Input.FilePattern = "["
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "input file pattern") {
		t.Fatalf("Validate() error = %v, want invalid input file pattern", err)
	}
}

func TestParseSettingsDefaultsMissingClaimSection(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	start := strings.Index(string(data), "        claims:\n")
	if start < 0 {
		t.Fatalf("generated settings Claim section not found:\n%s", data)
	}
	relativeEnd := strings.Index(string(data)[start:], "        extract_graph_prompt:")
	if relativeEnd < 0 {
		t.Fatalf("generated settings Claim section end not found:\n%s", data)
	}
	end := start + relativeEnd
	withoutClaims := append(append([]byte{}, data[:start]...), data[end:]...)
	parsed, err := ParseSettings(withoutClaims)
	if err != nil {
		t.Fatalf("ParseSettings(missing Claims) error = %v", err)
	}
	if parsed.Index.Standard.Claims != (ProjectClaimSettings{
		Enabled: false, Prompt: "prompts/extract_claims.txt",
		Description: extraction.DefaultClaimDescription, MaxGleanings: extraction.DefaultClaimMaxGleanings,
	}) {
		t.Fatalf("defaulted Claim settings = %#v", parsed.Index.Standard.Claims)
	}
}

func TestSettingsRejectsInvalidModelBaseURL(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	completion := settings.CompletionModels[settings.Index.CompletionModelID]
	completion.BaseURL = "completion.example/v1"
	settings.CompletionModels[settings.Index.CompletionModelID] = completion
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Fatalf("Validate() error = %v, want model base URL error", err)
	}
}

func TestSettingsValidateCompletionStructuredOutputMode(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatal(err)
	}
	completionID := settings.Index.CompletionModelID
	completion := settings.CompletionModels[completionID]
	completion.StructuredOutput = StructuredOutputJSONObject
	settings.CompletionModels[completionID] = completion
	if err := settings.Validate(); err != nil {
		t.Fatalf("Validate(json_object) error = %v", err)
	}
	completion.StructuredOutput = "xml"
	settings.CompletionModels[completionID] = completion
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "structured output") {
		t.Fatalf("Validate(xml) error = %v", err)
	}
	completion.StructuredOutput = StructuredOutputJSONObject
	settings.CompletionModels[completionID] = completion
	embeddingID := settings.Index.EmbeddingModelID
	embedding := settings.EmbeddingModels[embeddingID]
	embedding.StructuredOutput = StructuredOutputJSONObject
	settings.EmbeddingModels[embeddingID] = embedding
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "does not use structured output") {
		t.Fatalf("Validate(embedding structured output) error = %v", err)
	}
}

func TestParseSettingsDefaultsMissingCompletionStructuredOutput(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	withoutMode := strings.Replace(string(data), "        structured_output: json_schema\n", "", 1)
	parsed, err := ParseSettings([]byte(withoutMode))
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if got := parsed.CompletionModels[parsed.Index.CompletionModelID].StructuredOutput; got != StructuredOutputJSONSchema {
		t.Fatalf("default structured output = %q", got)
	}
}

func TestParseSettingsDefaultsMissingGlobalSearchSection(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	marker := "    global:\n"
	index := strings.Index(string(data), marker)
	if index < 0 {
		t.Fatalf("generated settings are missing %q", marker)
	}
	parsed, err := ParseSettings(data[:index])
	if err != nil {
		t.Fatalf("ParseSettings(missing global search) error = %v", err)
	}
	if got, want := parsed.Query.Global, defaultGlobalSearchConfig(); got != want {
		t.Fatalf("defaulted global search settings = %+v, want %+v", got, want)
	}
}

func TestSettingsRejectsInvalidGlobalSearchConfiguration(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*GlobalSearchConfig)
	}{
		{name: "context budget", mutate: func(config *GlobalSearchConfig) { config.MaxContextTokens = 0 }},
		{name: "reduce budget", mutate: func(config *GlobalSearchConfig) { config.DataMaxTokens = 0 }},
		{name: "map length", mutate: func(config *GlobalSearchConfig) { config.MapMaxLength = 0 }},
		{name: "reduce length", mutate: func(config *GlobalSearchConfig) { config.ReduceMaxLength = 0 }},
		{name: "dynamic threshold", mutate: func(config *GlobalSearchConfig) { config.DynamicSearchThreshold = -1 }},
		{name: "dynamic repeats", mutate: func(config *GlobalSearchConfig) { config.DynamicSearchNumRepeats = 0 }},
		{name: "dynamic max level", mutate: func(config *GlobalSearchConfig) { config.DynamicSearchMaxLevel = -1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := settings
			test.mutate(&candidate.Query.Global)
			if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), "global search") {
				t.Fatalf("Validate() error = %v, want global search error", err)
			}
		})
	}
}

func TestParseSettingsDefaultsMissingDriftSection(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	marker := "    drift:\n"
	index := strings.Index(string(data), marker)
	if index < 0 {
		t.Fatalf("generated settings are missing %q", marker)
	}
	parsed, err := ParseSettings(data[:index])
	if err != nil {
		t.Fatalf("ParseSettings(missing DRIFT) error = %v", err)
	}
	if got, want := parsed.Query.Drift, defaultDriftSearchConfig(); got != want {
		t.Fatalf("defaulted DRIFT settings = %+v, want %+v", got, want)
	}
}

func TestSettingsRejectsInvalidDriftConfiguration(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.Query.Drift.MaxModelCalls = 1
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "DRIFT search") {
		t.Fatalf("Validate() error = %v, want DRIFT search error", err)
	}
}

func TestParseSettingsDefaultsMissingReportVectorFields(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	withoutReportVectorFields := strings.ReplaceAll(string(data), "        enabled: true\n", "")
	withoutReportVectorFields = strings.ReplaceAll(withoutReportVectorFields, "        batch_size: 16\n", "")
	withoutReportVectorFields = strings.ReplaceAll(withoutReportVectorFields, "        batch_max_tokens: 8191\n", "")
	parsed, err := ParseSettings([]byte(withoutReportVectorFields))
	if err != nil {
		t.Fatalf("ParseSettings(missing embedding fields) error = %v", err)
	}
	if !parsed.Index.ReportVectors.Enabled ||
		parsed.Index.ReportVectors.BatchSize != semantic.DefaultBatchSize ||
		parsed.Index.ReportVectors.BatchMaxTokens != semantic.DefaultBatchMaxTokens {
		t.Fatalf("Report vector settings = %#v, want defaults", parsed.Index.ReportVectors)
	}
}

func TestParseSettingsDefaultsMissingConcurrentRequests(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	withoutConcurrency := strings.ReplaceAll(string(data), "concurrent_requests: 25\n", "")
	parsed, err := ParseSettings([]byte(withoutConcurrency))
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if parsed.ConcurrentRequests != DefaultConcurrentRequests {
		t.Fatalf("concurrent requests = %d, want %d", parsed.ConcurrentRequests, DefaultConcurrentRequests)
	}
}

func TestParseSettingsDefaultsMissingUseLCCButPreservesExplicitFalse(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	withoutUseLCC := strings.ReplaceAll(string(data), "        use_lcc: true\n", "")
	parsed, err := ParseSettings([]byte(withoutUseLCC))
	if err != nil {
		t.Fatalf("ParseSettings(missing use_lcc) error = %v", err)
	}
	if !parsed.Index.Community.UseLargestConnectedComponent {
		t.Fatal("missing use_lcc did not receive Python default true")
	}

	settings.Index.Community.UseLargestConnectedComponent = false
	data, err = MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings(false use_lcc) error = %v", err)
	}
	parsed, err = ParseSettings(data)
	if err != nil {
		t.Fatalf("ParseSettings(false use_lcc) error = %v", err)
	}
	if parsed.Index.Community.UseLargestConnectedComponent {
		t.Fatal("explicit use_lcc=false was replaced by the v2 default")
	}
}

func TestParseSettingsDefaultsMissingCommunityReportLimitsButPreservesExplicitZero(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	withoutLimits := strings.ReplaceAll(string(data), "        max_report_length: 2000\n", "")
	withoutLimits = strings.ReplaceAll(withoutLimits, "        max_report_input_tokens: 8000\n", "")
	parsed, err := ParseSettings([]byte(withoutLimits))
	if err != nil {
		t.Fatalf("ParseSettings(missing limits) error = %v", err)
	}
	if parsed.Index.Community.MaxReportLength != community.DefaultReportMaxLength ||
		parsed.Index.Community.MaxReportInputTokens != community.DefaultReportMaxInputTokens {
		t.Fatalf("missing community report limits = %d/%d, want defaults", parsed.Index.Community.MaxReportLength, parsed.Index.Community.MaxReportInputTokens)
	}

	settings.Index.Community.MaxReportLength = 0
	settings.Index.Community.MaxReportInputTokens = 0
	data, err = MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings(explicit zero) error = %v", err)
	}
	parsed, err = ParseSettings(data)
	if err != nil {
		t.Fatalf("ParseSettings(explicit zero) error = %v", err)
	}
	if parsed.Index.Community.MaxReportLength != 0 || parsed.Index.Community.MaxReportInputTokens != 0 {
		t.Fatalf("explicit community report limits = %d/%d, want 0/0", parsed.Index.Community.MaxReportLength, parsed.Index.Community.MaxReportInputTokens)
	}
}

func TestDefaultSettingsRejectsEmptyModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		completion string
		embedding  string
	}{
		{name: "completion", completion: " ", embedding: "embedding-test"},
		{name: "embedding", completion: "gpt-test", embedding: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DefaultSettings(test.completion, test.embedding); err == nil {
				t.Fatal("DefaultSettings() error = nil, want non-nil")
			}
		})
	}
}

func TestSettingsRejectsUnsafeStoragePaths(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Settings)
	}{
		{name: "empty cache", mutate: func(current *Settings) { current.Storage.CacheDir = "" }},
		{name: "escaping cache", mutate: func(current *Settings) { current.Storage.CacheDir = "../cache" }},
		{name: "absolute cache", mutate: func(current *Settings) { current.Storage.CacheDir = "/tmp/cache" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := settings
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want non-nil")
			}
		})
	}
}

func TestParseSettingsRejectsUnknownField(t *testing.T) {
	t.Parallel()

	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	data, err := MarshalSettings(settings)
	if err != nil {
		t.Fatalf("MarshalSettings() error = %v", err)
	}
	data = append(data, []byte("unknown_field: true\n")...)
	if _, err := ParseSettings(data); err == nil {
		t.Fatal("ParseSettings() error = nil, want non-nil")
	}
}

func TestSettingsRejectsInvalidReportVectorConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Settings)
	}{
		{name: "batch size", mutate: func(settings *Settings) { settings.Index.ReportVectors.BatchSize = 0 }},
		{name: "batch token limit", mutate: func(settings *Settings) { settings.Index.ReportVectors.BatchMaxTokens = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings, err := DefaultSettings("gpt-test", "embedding-test")
			if err != nil {
				t.Fatalf("DefaultSettings() error = %v", err)
			}
			test.mutate(&settings)
			if err := settings.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want non-nil")
			}
		})
	}
}

func TestSettingsRejectsInvalidDomainConcurrency(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.ConcurrentRequests = 0
	if err := settings.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want non-nil")
	}
}

func TestSettingsRejectsChunkOverlapEqualToChunkSize(t *testing.T) {
	settings, err := DefaultSettings("gpt-test", "embedding-test")
	if err != nil {
		t.Fatalf("DefaultSettings() error = %v", err)
	}
	settings.Chunking.Size = 100
	settings.Chunking.Overlap = 100
	if err := settings.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want non-nil")
	}
}
