package project

import (
	"maps"
	"strings"
	"testing"

	"github.com/memoria-space/meking/memory/activation"
	prompttext "github.com/memoria-space/meking/project/prompts"
)

func TestDefaultGraphExtractionPromptDeclaresRuntimeProtocol(t *testing.T) {
	prompt := DefaultPrompts()["extract_graph.txt"]
	if prompt != prompttext.GraphExtraction {
		t.Fatalf("extract graph prompt does not map to the graph extraction template")
	}
	for _, required := range []string{
		`{entity_types}`,
		`{input_text}`,
		`one JSON object with entities and relations arrays`,
		`name, type, aliases, and description`,
		`source, target, type, description, and weight`,
		`JSON array of strings`,
		`Use [] when no alternative name is supported`,
		`response schema supplied with the request`,
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("extract graph prompt does not contain %q", required)
		}
	}
	if !strings.HasPrefix(prompt, "<!-- meking-prompt-version: 5 -->\n") {
		t.Fatal("extract graph prompt has no version 5 header")
	}
}

func TestDefaultClaimExtractionPromptDeclaresRuntimeProtocol(t *testing.T) {
	prompt := DefaultPrompts()["extract_claims.txt"]
	if prompt != prompttext.ClaimExtraction {
		t.Fatal("extract Claim prompt does not map to the Claim extraction template")
	}
	for _, required := range []string{
		`{entity_specs}`,
		`{claim_description}`,
		`{input_text}`,
		`<subject_entity><|><object_entity><|><claim_type><|><claim_status>`,
		`Example 1:`,
		`Example 2:`,
		"##",
		"<|COMPLETE|>",
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("extract Claim prompt does not contain %q", required)
		}
	}
}

func TestDefaultCommunityReportPromptDeclaresRuntimeProtocol(t *testing.T) {
	prompt := DefaultPrompts()["community_report_graph.txt"]
	if prompt != prompttext.CommunityReportGraph {
		t.Fatalf("community report prompt does not map to the community report template")
	}
	for _, required := range []string{
		`{input_text}`,
		`{max_report_length}`,
		`"title"`,
		`"summary"`,
		`"findings"`,
		`"rating"`,
		`"rating_explanation"`,
		`[Data:`,
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("community report prompt does not contain %q", required)
		}
	}
}

func TestDefaultLocalSearchPromptDeclaresRuntimeProtocol(t *testing.T) {
	prompt := DefaultPrompts()["local_search_system_prompt.txt"]
	if prompt != prompttext.LocalSearchSystem {
		t.Fatalf("local search prompt does not map to the local answer template")
	}
	for _, required := range []string{
		`{context_data}`,
		`{response_type}`,
		`[Data:`,
		`Do not include information where the supporting evidence for it is not provided.`,
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("local search prompt does not contain %q", required)
		}
	}
}

func TestDefaultBasicSearchPromptDeclaresRuntimeProtocol(t *testing.T) {
	prompt := DefaultPrompts()["basic_search_system_prompt.txt"]
	if prompt != prompttext.BasicSearchSystem {
		t.Fatalf("basic search prompt does not map to the basic answer template")
	}
	for _, required := range []string{
		`{context_data}`,
		`{response_type}`,
		`[Data: Sources`,
		`Do not include information where the supporting evidence for it is not provided.`,
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("basic search prompt does not contain %q", required)
		}
	}
}

func TestDefaultQuestionGenerationPromptDeclaresRuntimeProtocol(t *testing.T) {
	prompt := DefaultPrompts()["question_gen_system_prompt.txt"]
	if prompt != prompttext.QuestionGenerationSystem {
		t.Fatal("question generation prompt does not map to the candidate template")
	}
	for _, required := range []string{
		`{context_data}`,
		`{question_count}`,
		`Use - marks as bullet points.`,
		`should be answerable using the data tables provided`,
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("question generation prompt does not contain %q", required)
		}
	}
}

func TestDriftHypotheticalPromptDeclaresRetrievalInputs(t *testing.T) {
	for _, required := range []string{"{query}", "{template}", "does not reference new named entities"} {
		if !strings.Contains(prompttext.DriftHypothetical, required) {
			t.Fatalf("DRIFT hypothetical prompt does not contain %q", required)
		}
	}
}

func TestDefaultGlobalReducePromptsDeclareRuntimeProtocol(t *testing.T) {
	reduce := DefaultPrompts()["global_search_reduce_system_prompt.txt"]
	if reduce != prompttext.GlobalSearchReduceSystem {
		t.Fatal("global reduce prompt does not map to the reduce template")
	}
	for _, required := range []string{
		`{report_data}`,
		`{response_type}`,
		`{max_length}`,
		`---Analyst Reports---`,
		`[Data: Reports`,
	} {
		if !strings.Contains(reduce, required) {
			t.Errorf("global reduce prompt does not contain %q", required)
		}
	}
	knowledge := DefaultPrompts()["global_search_knowledge_system_prompt.txt"]
	if knowledge != prompttext.GlobalSearchKnowledgeSystem ||
		!strings.Contains(knowledge, "[LLM: verify]") {
		t.Fatal("global knowledge prompt does not define the verification marker")
	}
}

func TestDefaultPromptsMapsProjectFilesToTemplates(t *testing.T) {
	want := map[string]string{
		"extract_graph.txt":                         prompttext.GraphExtraction,
		"extract_claims.txt":                        prompttext.ClaimExtraction,
		"community_report_graph.txt":                prompttext.CommunityReportGraph,
		"community_report_text.txt":                 prompttext.CommunityReportText,
		"drift_search_system_prompt.txt":            prompttext.DriftSearchSystem,
		"drift_reduce_prompt.txt":                   prompttext.DriftReduce,
		"global_search_map_system_prompt.txt":       prompttext.GlobalSearchMapSystem,
		"global_search_reduce_system_prompt.txt":    prompttext.GlobalSearchReduceSystem,
		"global_search_knowledge_system_prompt.txt": prompttext.GlobalSearchKnowledgeSystem,
		"local_search_system_prompt.txt":            prompttext.LocalSearchSystem,
		"basic_search_system_prompt.txt":            prompttext.BasicSearchSystem,
		"question_gen_system_prompt.txt":            prompttext.QuestionGenerationSystem,
	}

	maps.Copy(want, activation.DefaultPrompts())
	if got := DefaultPrompts(); !maps.Equal(got, want) {
		t.Fatalf("default prompts = %#v, want %#v", got, want)
	}
}

func TestDefaultPromptsReturnsFreshMap(t *testing.T) {
	first := DefaultPrompts()
	first["extract_graph.txt"] = "changed"

	if got := DefaultPrompts()["extract_graph.txt"]; got != prompttext.GraphExtraction {
		t.Fatalf("extract graph prompt after caller mutation = %q", got)
	}
}
