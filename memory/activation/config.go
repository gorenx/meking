package activation

import (
	"fmt"
	"path/filepath"

	"github.com/memoria-space/meking/knowledge/mas"
)

type Config struct {
	// Parameters overrides all 21 model weights in index order. Omission uses
	// the domain defaults; partial lists are invalid. Changing them does not
	// migrate existing memory states. Index meanings are documented by mas.Config.
	Parameters []float64 `yaml:"parameters,omitempty"`
	// Empty paths use built-in bilingual instructions, allowing older Projects
	// to start without creating prompt files during configuration loading.
	ChinesePrompt string `yaml:"chinese_prompt,omitempty"`
	EnglishPrompt string `yaml:"english_prompt,omitempty"`
}

// Configuration is the resolved startup configuration. It has no file handles
// or dependency on Project; all values are fixed before service construction.
type Configuration struct {
	Model         mas.Config
	ChinesePrompt string
	EnglishPrompt string
}

func (settings Config) model() (mas.Config, error) {
	config := mas.DefaultConfig()
	if settings.Parameters != nil {
		if len(settings.Parameters) != len(config.Parameters) {
			return mas.Config{}, fmt.Errorf("activation parameters require exactly %d values", len(config.Parameters))
		}
		copy(config.Parameters[:], settings.Parameters)
	}
	if err := config.Validate(); err != nil {
		return mas.Config{}, fmt.Errorf("validate activation parameters: %w", err)
	}
	return config, nil
}

func (settings Config) Validate() error {
	if _, err := settings.model(); err != nil {
		return err
	}
	for _, path := range settings.PromptFiles() {
		if !validText(path) || !filepath.IsLocal(path) || filepath.Clean(path) == "." {
			return fmt.Errorf("activation: prompt must name a local relative file")
		}
	}
	return nil
}

// PromptFiles declares the external assets the configuration loader must read.
func (settings Config) PromptFiles() []string {
	var paths []string
	for _, path := range []string{settings.ChinesePrompt, settings.EnglishPrompt} {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// Resolve applies domain defaults and validates the exact loaded prompt text.
// File IO and Project root confinement remain the loader's responsibility.
func (settings Config) Resolve(prompts map[string]string) (Configuration, error) {
	if err := settings.Validate(); err != nil {
		return Configuration{}, err
	}
	model, err := settings.model()
	if err != nil {
		return Configuration{}, err
	}
	result := Configuration{Model: model, ChinesePrompt: recallChinese, EnglishPrompt: recallEnglish}
	if settings.ChinesePrompt != "" {
		result.ChinesePrompt = prompts[settings.ChinesePrompt]
	}
	if settings.EnglishPrompt != "" {
		result.EnglishPrompt = prompts[settings.EnglishPrompt]
	}
	if !validText(result.ChinesePrompt) || !validText(result.EnglishPrompt) {
		return Configuration{}, fmt.Errorf("activation: recall evaluation prompts must be valid nonempty text")
	}
	return result, nil
}
