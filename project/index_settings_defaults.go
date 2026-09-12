package project

import (
	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/knowledge/extraction"
	"github.com/memoria-space/meking/semantic"
)

// defaultIndexSettings composes domain-owned defaults into the persisted
// Project representation. Project owns paths and schema layout, not the
// algorithm values copied into that representation.
func defaultIndexSettings() ProjectIndexSettings {
	detection := community.DefaultDetectConfig()
	return ProjectIndexSettings{
		CompletionModelID: "default_completion_model",
		EmbeddingModelID:  "default_embedding_model",
		Community: ProjectCommunitySettings{
			MaxSize: detection.MaxClusterSize, Seed: uint64(detection.Seed),
			UseLargestConnectedComponent: detection.UseLargestConnectedComponent,
			RelationChangeThreshold:      community.DefaultRelationChangeThreshold,
			MaxReportLength:              community.DefaultReportMaxLength,
			MaxReportInputTokens:         community.DefaultReportMaxInputTokens,
		},
		ReportVectors: ProjectReportVectorSettings{
			Enabled:   true,
			BatchSize: semantic.DefaultBatchSize, BatchMaxTokens: semantic.DefaultBatchMaxTokens,
		},
		Standard: defaultStandardIndexSettings(),
	}
}

func defaultStandardIndexSettings() ProjectStandardSettings {
	graphPolicy := extraction.DefaultGraphExtractionConfig()
	claims := extraction.DefaultClaimExtractionConfig()
	return ProjectStandardSettings{
		EntityTypes:  append([]string(nil), graphPolicy.EntityTypes...),
		MaxGleanings: graphPolicy.MaxGleanings,
		Claims: ProjectClaimSettings{
			Prompt: "prompts/extract_claims.txt", Description: claims.Description,
			MaxGleanings: claims.MaxGleanings,
		},
		ExtractGraphPrompt:    "prompts/extract_graph.txt",
		CommunityReportPrompt: "prompts/community_report_graph.txt",
	}
}
