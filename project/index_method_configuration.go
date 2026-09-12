package project

import (
	"errors"
	"fmt"
	"math"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/knowledge/extraction"
	"github.com/memoria-space/meking/semantic"
)

func (configuration Configuration) Completion() ModelConfig {
	return configuration.completionModel
}

func (configuration Configuration) Embedding() ModelConfig {
	return configuration.embeddingModel
}

func (configuration Configuration) QueryCompletion() ModelConfig {
	return configuration.queryModel
}

// CommunityConfiguration contains only Community-owned policy. Concrete model
// and vector services are connected later by the Project composition root.
type CommunityConfiguration struct {
	Detection               community.DetectConfig
	RelationChangeThreshold uint64
	Reports                 communityreport.Config
	ReportVectors           *semantic.GenerationConfig
}

func newConfiguration(settings Settings, prompts map[string]string) (Configuration, error) {
	activation, err := settings.Activation.Resolve(prompts)
	if err != nil {
		return Configuration{}, err
	}
	chunking := textunits.Chunking{
		Type:          textunits.ChunkingType(settings.Chunking.Type),
		Size:          settings.Chunking.Size,
		Overlap:       settings.Chunking.Overlap,
		EncodingModel: settings.Chunking.EncodingModel,
	}
	if chunking.Type == textunits.SentenceChunking {
		chunking.SentenceLanguage = textunits.DefaultSentenceLanguage
	}
	if err := textunits.ValidateChunking(chunking); err != nil {
		return Configuration{}, fmt.Errorf("validate Corpus chunking configuration: %w", err)
	}
	detection, err := projectCommunityDetectionConfig(settings.Index.Community)
	if err != nil {
		return Configuration{}, err
	}
	extractionPolicy := extraction.Policy{
		Graph: projectGraphExtractionConfig(
			settings,
			prompts[settings.Index.Standard.ExtractGraphPrompt],
		),
	}
	if settings.Index.Standard.Claims.Enabled {
		claims := projectClaimExtractionConfig(
			settings,
			prompts[settings.Index.Standard.Claims.Prompt],
		)
		extractionPolicy.Claims = &claims
	}
	textUnitVectors, err := projectTextUnitEmbeddingGenerationConfig(settings)
	if err != nil {
		return Configuration{}, err
	}
	entityVectors, err := projectEntityEmbeddingGenerationConfig(settings)
	if err != nil {
		return Configuration{}, err
	}
	communityConfiguration := CommunityConfiguration{
		Detection:               detection,
		RelationChangeThreshold: settings.Index.Community.RelationChangeThreshold,
		Reports: projectStandardReportConfig(
			settings,
			prompts[settings.Index.Standard.CommunityReportPrompt],
		),
	}
	if settings.Index.ReportVectors.Enabled {
		reportVectors, err := projectReportEmbeddingGenerationConfig(settings)
		if err != nil {
			return Configuration{}, err
		}
		communityConfiguration.ReportVectors = &reportVectors
	}
	configuration := Configuration{
		Activation:     activation,
		Input:          settings.Input,
		CacheDirectory: settings.Storage.CacheDir,
		MaxConcurrent:  settings.ConcurrentRequests,
		Query: QueryConfiguration{
			GlobalMapPrompt:       prompts[settings.Query.GlobalSearchMapPrompt],
			GlobalReducePrompt:    prompts[settings.Query.GlobalSearchReducePrompt],
			GlobalKnowledgePrompt: prompts[settings.Query.GlobalSearchKnowledgePrompt],
			Global:                settings.Query.Global,
			DriftSearchPrompt:     prompts[settings.Query.DriftSearchPrompt],
			DriftReducePrompt:     prompts[settings.Query.DriftReducePrompt],
			Drift:                 driftConfiguration(settings.Query.Drift),
		},
		completionModel: settings.CompletionModels[settings.Index.CompletionModelID],
		embeddingModel:  settings.EmbeddingModels[settings.Index.EmbeddingModelID],
		queryModel:      settings.CompletionModels[settings.Query.CompletionModelID],
		Chunking:        chunking,
		Extraction:      extractionPolicy,
		Community:       communityConfiguration,
		TextUnitVectors: textUnitVectors,
		EntityVectors:   entityVectors,
	}
	if err := validateConfiguration(configuration); err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}

func validateConfiguration(configuration Configuration) error {
	if err := configuration.Extraction.Validate(); err != nil {
		return fmt.Errorf("validate Knowledge extraction configuration: %w", err)
	}
	if err := configuration.Community.Detection.Validate(); err != nil {
		return fmt.Errorf("validate Community detection configuration: %w", err)
	}
	if configuration.Community.RelationChangeThreshold == 0 {
		return errors.New("Community relation change threshold must be positive")
	}
	if err := configuration.Community.Reports.Validate(); err != nil {
		return fmt.Errorf("validate Community report configuration: %w", err)
	}
	if err := configuration.TextUnitVectors.Validate(); err != nil {
		return fmt.Errorf("validate TextUnit vector configuration: %w", err)
	}
	if err := configuration.EntityVectors.Validate(); err != nil {
		return fmt.Errorf("validate Entity vector configuration: %w", err)
	}
	if configuration.Community.ReportVectors != nil {
		if err := configuration.Community.ReportVectors.Validate(); err != nil {
			return fmt.Errorf("validate Report vector configuration: %w", err)
		}
	}
	if err := configuration.Query.Drift.Validate(); err != nil {
		return fmt.Errorf("validate DRIFT query configuration: %w", err)
	}
	return nil
}

// validateProjectIndexPolicies maps persisted fields into their owning
// domain configurations and delegates semantic validation to those domains.
// It performs no filesystem access and does not resolve prompt paths.
func validateProjectIndexPolicies(settings Settings) error {
	detection, err := projectCommunityDetectionConfig(settings.Index.Community)
	if err != nil {
		return err
	}
	if err := detection.Validate(); err != nil {
		return fmt.Errorf("validate community detection settings: %w", err)
	}
	textUnitEmbeddings, err := projectTextUnitEmbeddingGenerationConfig(settings)
	if err != nil {
		return err
	}
	if err := textUnitEmbeddings.Validate(); err != nil {
		return fmt.Errorf("validate TextUnit embedding settings: %w", err)
	}
	entityEmbeddings, err := projectEntityEmbeddingGenerationConfig(settings)
	if err != nil {
		return err
	}
	if err := entityEmbeddings.Validate(); err != nil {
		return fmt.Errorf("validate Entity embedding settings: %w", err)
	}
	if settings.Index.ReportVectors.Enabled {
		reportEmbeddings, err := projectReportEmbeddingGenerationConfig(settings)
		if err != nil {
			return err
		}
		if err := reportEmbeddings.Validate(); err != nil {
			return fmt.Errorf("validate Report embedding settings: %w", err)
		}
	}
	if err := projectGraphExtractionConfig(settings, "").Validate(); err != nil {
		return fmt.Errorf("validate Standard graph extraction settings: %w", err)
	}
	if err := projectClaimExtractionConfig(settings, "").Validate(); err != nil {
		return fmt.Errorf("validate Standard Claim settings: %w", err)
	}
	return nil
}

func projectGraphExtractionConfig(settings Settings, prompt string) extraction.GraphExtractionConfig {
	return extraction.GraphExtractionConfig{
		Prompt: prompt, EntityTypes: append([]string(nil), settings.Index.Standard.EntityTypes...),
		MaxGleanings: settings.Index.Standard.MaxGleanings,
	}
}

func projectClaimExtractionConfig(settings Settings, prompt string) extraction.ClaimExtractionConfig {
	return extraction.ClaimExtractionConfig{
		Prompt: prompt, EntityTypes: append([]string(nil), settings.Index.Standard.EntityTypes...),
		Description:  settings.Index.Standard.Claims.Description,
		MaxGleanings: settings.Index.Standard.Claims.MaxGleanings,
	}
}

func projectCommunityDetectionConfig(settings ProjectCommunitySettings) (community.DetectConfig, error) {
	if settings.Seed > math.MaxInt64 {
		return community.DetectConfig{}, errors.New("community seed exceeds the supported signed range")
	}
	return community.DetectConfig{
		MaxClusterSize:               settings.MaxSize,
		UseLargestConnectedComponent: settings.UseLargestConnectedComponent,
		Seed:                         int64(settings.Seed),
	}, nil
}

func projectStandardReportConfig(settings Settings, prompt string) communityreport.Config {
	completion := settings.CompletionModels[settings.Index.CompletionModelID]
	return communityreport.Config{
		Prompt:          prompt,
		Model:           completion.Model,
		Tokenizer:       settings.Chunking.EncodingModel,
		MaxInputTokens:  settings.Index.Community.MaxReportInputTokens,
		MaxReportLength: settings.Index.Community.MaxReportLength,
		MaxConcurrency:  settings.ConcurrentRequests,
	}
}

func projectTextUnitEmbeddingGenerationConfig(settings Settings) (semantic.GenerationConfig, error) {
	_, found := settings.EmbeddingModels[settings.Index.EmbeddingModelID]
	if !found {
		return semantic.GenerationConfig{}, fmt.Errorf(
			"unknown embedding model id %q", settings.Index.EmbeddingModelID,
		)
	}
	configuration := semantic.DefaultGenerationConfig()
	configuration.MaxConcurrency = settings.ConcurrentRequests
	return configuration, nil
}

func projectEntityEmbeddingGenerationConfig(settings Settings) (semantic.GenerationConfig, error) {
	_, found := settings.EmbeddingModels[settings.Index.EmbeddingModelID]
	if !found {
		return semantic.GenerationConfig{}, fmt.Errorf(
			"unknown embedding model id %q", settings.Index.EmbeddingModelID,
		)
	}
	configuration := semantic.DefaultGenerationConfig()
	configuration.MaxConcurrency = settings.ConcurrentRequests
	return configuration, nil
}

func projectReportEmbeddingGenerationConfig(settings Settings) (semantic.GenerationConfig, error) {
	_, found := settings.EmbeddingModels[settings.Index.EmbeddingModelID]
	if !found {
		return semantic.GenerationConfig{}, fmt.Errorf(
			"unknown embedding model id %q", settings.Index.EmbeddingModelID,
		)
	}
	return semantic.GenerationConfig{
		BatchSize:      settings.Index.ReportVectors.BatchSize,
		BatchMaxTokens: settings.Index.ReportVectors.BatchMaxTokens,
		MaxConcurrency: settings.ConcurrentRequests,
	}, nil
}
