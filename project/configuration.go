package project

import (
	"fmt"
	"sort"

	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/knowledge/extraction"
	"github.com/memoria-space/meking/memory/activation"
	querydrift "github.com/memoria-space/meking/query/drift"
	"github.com/memoria-space/meking/semantic"
)

// QueryConfiguration contains the Query policy and Prompt contents fixed when
// the Project process starts.
type QueryConfiguration struct {
	GlobalMapPrompt       string
	GlobalReducePrompt    string
	GlobalKnowledgePrompt string
	Global                GlobalSearchConfig
	DriftSearchPrompt     string
	DriftReducePrompt     string
	Drift                 querydrift.Configuration
}

// Configuration is the single validated configuration loaded for one
// Project process. It is never refreshed while that process is running.
type Configuration struct {
	Input           InputConfig
	CacheDirectory  string
	MaxConcurrent   int
	Query           QueryConfiguration
	completionModel ModelConfig
	embeddingModel  ModelConfig
	queryModel      ModelConfig
	Chunking        textunits.Chunking
	Extraction      extraction.Policy
	Community       CommunityConfiguration
	TextUnitVectors semantic.GenerationConfig
	EntityVectors   semantic.GenerationConfig
	Activation      activation.Configuration
}

func LoadConfiguration(projectRoot string) (Configuration, error) {
	root, err := existingDirectory(projectRoot, "project")
	if err != nil {
		return Configuration{}, err
	}
	settingsData, err := readRegularProjectFile(root, "settings.yaml")
	if err != nil {
		return Configuration{}, fmt.Errorf("read Project settings: %w", err)
	}
	settings, err := ParseSettings(settingsData)
	if err != nil {
		return Configuration{}, err
	}
	credentials := loadModelCredentials(root)
	settings, err = credentials.resolveSettings(settings)
	if err != nil {
		return Configuration{}, err
	}
	prompts, err := loadProjectPrompts(root, settings)
	if err != nil {
		return Configuration{}, err
	}
	configuration, err := newConfiguration(settings, prompts)
	if err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}

func loadProjectPrompts(root string, settings Settings) (map[string]string, error) {
	paths := []string{
		settings.Index.Standard.ExtractGraphPrompt,
		settings.Index.Standard.CommunityReportPrompt,
		settings.Query.GlobalSearchMapPrompt,
		settings.Query.GlobalSearchReducePrompt,
		settings.Query.GlobalSearchKnowledgePrompt,
		settings.Query.DriftSearchPrompt,
		settings.Query.DriftReducePrompt,
	}
	if settings.Index.Standard.Claims.Enabled {
		paths = append(paths, settings.Index.Standard.Claims.Prompt)
	}
	paths = append(paths, settings.Activation.PromptFiles()...)
	sort.Strings(paths)
	prompts := make(map[string]string, len(paths))
	for _, path := range paths {
		if _, exists := prompts[path]; exists {
			continue
		}
		content, err := readRegularProjectFile(root, path)
		if err != nil {
			return nil, fmt.Errorf("read Project Prompt %q: %w", path, err)
		}
		prompts[path] = string(content)
	}
	return prompts, nil
}
