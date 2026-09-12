package project

import (
	"github.com/memoria-space/meking/memory/activation"
	prompttext "github.com/memoria-space/meking/project/prompts"
	"maps"
)

// DefaultPrompts returns a fresh map of versioned Project prompt templates.
func DefaultPrompts() map[string]string {
	result := map[string]string{
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
	maps.Copy(result, activation.DefaultPrompts())
	return result
}
