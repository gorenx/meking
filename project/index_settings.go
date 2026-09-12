package project

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// ProjectIndexSettings is the YAML representation of Standard indexing. It
// contains Project references and user-visible policy, then composition maps
// those values into domain-owned configurations.
type ProjectIndexSettings struct {
	CompletionModelID string                      `yaml:"completion_model_id"`
	EmbeddingModelID  string                      `yaml:"embedding_model_id"`
	Community         ProjectCommunitySettings    `yaml:"community"`
	ReportVectors     ProjectReportVectorSettings `yaml:"report_vectors"`
	Standard          ProjectStandardSettings     `yaml:"standard"`
}

// ProjectCommunitySettings persists hierarchy and report-generation policy.
type ProjectCommunitySettings struct {
	MaxSize                      int    `yaml:"max_size"`
	Seed                         uint64 `yaml:"seed"`
	UseLargestConnectedComponent bool   `yaml:"use_lcc"`
	RelationChangeThreshold      uint64 `yaml:"relation_change_threshold"`
	MaxReportLength              int    `yaml:"max_report_length"`
	MaxReportInputTokens         int    `yaml:"max_report_input_tokens"`
}

// ProjectReportVectorSettings controls the optional Report vector Namespace.
type ProjectReportVectorSettings struct {
	Enabled        bool `yaml:"enabled"`
	BatchSize      int  `yaml:"batch_size"`
	BatchMaxTokens int  `yaml:"batch_max_tokens"`
}

// ProjectStandardSettings persists only the LLM graph extraction, optional
// Claim, and graph-report fields of Standard indexing.
type ProjectStandardSettings struct {
	EntityTypes           []string             `yaml:"entity_types"`
	MaxGleanings          int                  `yaml:"max_gleanings"`
	Claims                ProjectClaimSettings `yaml:"claims"`
	ExtractGraphPrompt    string               `yaml:"extract_graph_prompt"`
	CommunityReportPrompt string               `yaml:"community_report_prompt"`
}

// ProjectClaimSettings defines the user-visible Standard Claim extraction policy
// without exposing model transport or storage choices.
type ProjectClaimSettings struct {
	Enabled      bool   `yaml:"enabled"`
	Prompt       string `yaml:"prompt"`
	Description  string `yaml:"description"`
	MaxGleanings int    `yaml:"max_gleanings"`
}

// projectIndexFieldPresence distinguishes omitted fields from explicit zero,
// false, empty-list, and space-delimiter values while applying same-version
// defaults.
type projectIndexFieldPresence struct {
	Community *struct {
		MaxSize                      *int    `yaml:"max_size"`
		Seed                         *uint64 `yaml:"seed"`
		UseLargestConnectedComponent *bool   `yaml:"use_lcc"`
		RelationChangeThreshold      *uint64 `yaml:"relation_change_threshold"`
		MaxReportLength              *int    `yaml:"max_report_length"`
		MaxReportInputTokens         *int    `yaml:"max_report_input_tokens"`
	} `yaml:"community"`
	ReportVectors *struct {
		Enabled        *bool `yaml:"enabled"`
		BatchSize      *int  `yaml:"batch_size"`
		BatchMaxTokens *int  `yaml:"batch_max_tokens"`
	} `yaml:"report_vectors"`
	Standard *struct {
		EntityTypes           *[]string `yaml:"entity_types"`
		MaxGleanings          *int      `yaml:"max_gleanings"`
		ExtractGraphPrompt    *string   `yaml:"extract_graph_prompt"`
		CommunityReportPrompt *string   `yaml:"community_report_prompt"`
		Claims                *struct {
			Enabled      *bool   `yaml:"enabled"`
			Prompt       *string `yaml:"prompt"`
			Description  *string `yaml:"description"`
			MaxGleanings *int    `yaml:"max_gleanings"`
		} `yaml:"claims"`
	} `yaml:"standard"`
}

func applyIndexSettingsDefaults(presence projectIndexFieldPresence, settings *ProjectIndexSettings) {
	defaults := defaultIndexSettings()
	if presence.Community == nil {
		settings.Community = defaults.Community
	} else {
		if presence.Community.MaxSize == nil {
			settings.Community.MaxSize = defaults.Community.MaxSize
		}
		if presence.Community.Seed == nil {
			settings.Community.Seed = defaults.Community.Seed
		}
		if presence.Community.UseLargestConnectedComponent == nil {
			settings.Community.UseLargestConnectedComponent = defaults.Community.UseLargestConnectedComponent
		}
		if presence.Community.RelationChangeThreshold == nil {
			settings.Community.RelationChangeThreshold = defaults.Community.RelationChangeThreshold
		}
		if presence.Community.MaxReportLength == nil {
			settings.Community.MaxReportLength = defaults.Community.MaxReportLength
		}
		if presence.Community.MaxReportInputTokens == nil {
			settings.Community.MaxReportInputTokens = defaults.Community.MaxReportInputTokens
		}
	}
	if presence.ReportVectors == nil {
		settings.ReportVectors = defaults.ReportVectors
	} else {
		if presence.ReportVectors.Enabled == nil {
			settings.ReportVectors.Enabled = defaults.ReportVectors.Enabled
		}
		if presence.ReportVectors.BatchSize == nil {
			settings.ReportVectors.BatchSize = defaults.ReportVectors.BatchSize
		}
		if presence.ReportVectors.BatchMaxTokens == nil {
			settings.ReportVectors.BatchMaxTokens = defaults.ReportVectors.BatchMaxTokens
		}
	}
	if presence.Standard == nil {
		settings.Standard = defaults.Standard
	} else {
		if presence.Standard.EntityTypes == nil {
			settings.Standard.EntityTypes = defaults.Standard.EntityTypes
		}
		if presence.Standard.MaxGleanings == nil {
			settings.Standard.MaxGleanings = defaults.Standard.MaxGleanings
		}
		if presence.Standard.ExtractGraphPrompt == nil {
			settings.Standard.ExtractGraphPrompt = defaults.Standard.ExtractGraphPrompt
		}
		if presence.Standard.CommunityReportPrompt == nil {
			settings.Standard.CommunityReportPrompt = defaults.Standard.CommunityReportPrompt
		}
		if presence.Standard.Claims == nil {
			settings.Standard.Claims = defaults.Standard.Claims
		} else {
			if presence.Standard.Claims.Prompt == nil {
				settings.Standard.Claims.Prompt = defaults.Standard.Claims.Prompt
			}
			if presence.Standard.Claims.Description == nil {
				settings.Standard.Claims.Description = defaults.Standard.Claims.Description
			}
			if presence.Standard.Claims.MaxGleanings == nil {
				settings.Standard.Claims.MaxGleanings = defaults.Standard.Claims.MaxGleanings
			}
		}
	}
}

func (c ProjectIndexSettings) validateStructure() error {
	if strings.TrimSpace(c.CompletionModelID) == "" {
		return errors.New("index completion model id is required")
	}
	if strings.TrimSpace(c.EmbeddingModelID) == "" {
		return errors.New("index embedding model id is required")
	}
	if c.Community.RelationChangeThreshold == 0 ||
		c.Community.RelationChangeThreshold > uint64(math.MaxInt64) {
		return errors.New("community relation change threshold must fit a positive signed integer")
	}
	if c.ReportVectors.BatchSize <= 0 || c.ReportVectors.BatchMaxTokens <= 0 {
		return errors.New("Report vector batch limits must be positive")
	}
	if err := c.Standard.validateStructure(); err != nil {
		return fmt.Errorf("validate Standard index settings: %w", err)
	}
	return nil
}

func (c ProjectStandardSettings) validateStructure() error {
	for label, path := range map[string]string{
		"graph extraction prompt": c.ExtractGraphPrompt,
		"community report prompt": c.CommunityReportPrompt,
		"claim extraction prompt": c.Claims.Prompt,
	} {
		if err := validateConfiguredRelativePath(path, label); err != nil {
			return err
		}
	}
	if strings.TrimSpace(c.Claims.Description) == "" {
		return errors.New("claim description is required")
	}
	return nil
}
