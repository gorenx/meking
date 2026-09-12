package project

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/memoria-space/meking/memory/activation"

	querydrift "github.com/memoria-space/meking/query/drift"
	queryglobal "github.com/memoria-space/meking/query/global"
	"go.yaml.in/yaml/v3"
)

const (
	// CurrentSettingsVersion identifies the Standard-only Project settings schema.
	CurrentSettingsVersion = 5
	// DefaultCompletionModel is the first-release completion model.
	DefaultCompletionModel = "gpt-4.1"
	// DefaultEmbeddingModel is the first-release embedding model.
	DefaultEmbeddingModel = "text-embedding-3-large"
	// StructuredOutputJSONSchema selects provider-enforced JSON Schema output.
	StructuredOutputJSONSchema = "json_schema"
	// StructuredOutputJSONObject selects valid JSON with application-side contract validation.
	StructuredOutputJSONObject = "json_object"
	// DefaultConcurrentRequests is the shared domain-operation concurrency limit.
	DefaultConcurrentRequests = 25
	// DefaultRichDocumentMaxBytes is the per-file policy used when a rich input
	// Project omits an explicit bound.
	DefaultRichDocumentMaxBytes int64 = 32 << 20
	// MaximumRichDocumentMaxBytes matches the fixed local transport hard limit.
	MaximumRichDocumentMaxBytes int64 = 64 << 20
	// InputTypeText selects the existing UTF-8 text reader.
	InputTypeText = "text"
	// InputTypeRichDocuments selects rich document conversion.
	InputTypeRichDocuments = "markitdown"
)

// Settings is the versioned Project configuration.
type Settings struct {
	Version int `yaml:"version"`
	// ConcurrentRequests limits concurrent external work scheduled by domains.
	ConcurrentRequests int                    `yaml:"concurrent_requests"`
	CompletionModels   map[string]ModelConfig `yaml:"completion_models"`
	EmbeddingModels    map[string]ModelConfig `yaml:"embedding_models"`
	Input              InputConfig            `yaml:"input"`
	Chunking           ChunkingConfig         `yaml:"chunking"`
	Storage            StorageConfig          `yaml:"storage"`
	Index              ProjectIndexSettings   `yaml:"index"`
	Query              QueryConfig            `yaml:"query"`
	Activation         activation.Config      `yaml:"activation,omitempty"`
}

// ModelConfig is the persisted Agent endpoint configuration for one named capability.
// Project loading resolves its credential reference before composition roots
// pass the model and endpoint to agent.
type ModelConfig struct {
	Provider string `yaml:"model_provider"`
	Model    string `yaml:"model"`
	// BaseURL selects the provider API root. Empty uses the provider default.
	BaseURL string `yaml:"base_url"`
	// StructuredOutput applies only to Completion models. Embedding models do
	// not use a response-format protocol.
	StructuredOutput string `yaml:"structured_output,omitempty"`
	AuthMethod       string `yaml:"auth_method"`
	APIKey           string `yaml:"api_key"`
}

// InputConfig selects one confirmed local corpus input strategy.
type InputConfig struct {
	Type        string `yaml:"type"`
	BaseDir     string `yaml:"base_dir"`
	FilePattern string `yaml:"file_pattern,omitempty"`
	Encoding    string `yaml:"encoding,omitempty"`
	// MaxFileBytes applies only to rich binary documents. Zero selects the
	// bounded default; text input has no corresponding byte-limit setting.
	MaxFileBytes int64 `yaml:"max_file_bytes,omitempty"`
}

// ChunkingConfig selects token-window or one-sentence text unit creation.
// Size and Overlap apply only to the tokens strategy; EncodingModel is used
// by both strategies to count final TextUnits.
type ChunkingConfig struct {
	Type          string `yaml:"type"`
	Size          int    `yaml:"size"`
	Overlap       int    `yaml:"overlap"`
	EncodingModel string `yaml:"encoding_model"`
}

// StorageConfig names Project-local storage locations.
type StorageConfig struct {
	CacheDir string `yaml:"cache_dir"`
}

// QueryConfig selects the model, prompt assets, and Query-owned policies.
type QueryConfig struct {
	CompletionModelID           string             `yaml:"completion_model_id"`
	GlobalSearchMapPrompt       string             `yaml:"global_search_map_prompt"`
	GlobalSearchReducePrompt    string             `yaml:"global_search_reduce_prompt"`
	GlobalSearchKnowledgePrompt string             `yaml:"global_search_knowledge_prompt"`
	DriftSearchPrompt           string             `yaml:"drift_search_prompt"`
	DriftReducePrompt           string             `yaml:"drift_reduce_prompt"`
	Global                      GlobalSearchConfig `yaml:"global"`
	Drift                       DriftSearchConfig  `yaml:"drift"`
}

// DriftSearchConfig persists the bounded global-to-local query policy.
type DriftSearchConfig struct {
	CommunityLevel            int    `yaml:"community_level"`
	Reports                   int    `yaml:"reports"`
	PrimerFolds               int    `yaml:"primer_folds"`
	BatchSize                 int    `yaml:"batch_size"`
	FollowUpLimit             int    `yaml:"follow_up_limit"`
	MaxDepth                  int    `yaml:"max_depth"`
	MaxBranches               int    `yaml:"max_branches"`
	MaxConcurrency            int    `yaml:"max_concurrency"`
	MaxModelCalls             int    `yaml:"max_model_calls"`
	MaxPromptTokens           int    `yaml:"max_prompt_tokens"`
	MaxOutputTokens           int    `yaml:"max_output_tokens"`
	MaxDurationSeconds        int    `yaml:"max_duration_seconds"`
	PrimerMaxPromptTokens     int    `yaml:"primer_max_prompt_tokens"`
	HyDEMaxCompletionTokens   int    `yaml:"hyde_max_completion_tokens"`
	PrimerMaxCompletionTokens int    `yaml:"primer_max_completion_tokens"`
	BranchMaxCompletionTokens int    `yaml:"branch_max_completion_tokens"`
	ReduceMaxContextTokens    int    `yaml:"reduce_max_context_tokens"`
	ReduceMaxCompletionTokens int    `yaml:"reduce_max_completion_tokens"`
	ResponseType              string `yaml:"response_type"`
}

// GlobalSearchConfig defines Project defaults for turning community reports
// into map inputs, ranked reduce evidence, and optional hierarchy traversal.
type GlobalSearchConfig struct {
	// MaxContextTokens budgets each report table and, independently, conversation history.
	MaxContextTokens int `yaml:"max_context_tokens"`
	// DataMaxTokens is the token budget for ranked Map points sent to Reduce.
	DataMaxTokens int `yaml:"data_max_tokens"`
	// MapMaxLength is the requested word count for each intermediate response.
	MapMaxLength int `yaml:"map_max_length"`
	// ReduceMaxLength is the requested word count for the final response.
	ReduceMaxLength int `yaml:"reduce_max_length"`
	// DynamicSearchThreshold accepts ratings at or above this value.
	DynamicSearchThreshold int `yaml:"dynamic_search_threshold"`
	// DynamicSearchKeepParent retains a relevant parent beside relevant children.
	DynamicSearchKeepParent bool `yaml:"dynamic_search_keep_parent"`
	// DynamicSearchNumRepeats controls sequential votes per visited report.
	DynamicSearchNumRepeats int `yaml:"dynamic_search_num_repeats"`
	// DynamicSearchUseSummary rates report summaries instead of full content.
	DynamicSearchUseSummary bool `yaml:"dynamic_search_use_summary"`
	// DynamicSearchMaxLevel limits fallback when a level has no relevant report.
	DynamicSearchMaxLevel int `yaml:"dynamic_search_max_level"`
}

// DefaultSettings builds a validated first-release configuration.
func DefaultSettings(completionModel, embeddingModel string) (Settings, error) {
	completionModel = strings.TrimSpace(completionModel)
	embeddingModel = strings.TrimSpace(embeddingModel)
	if completionModel == "" {
		return Settings{}, errors.New("completion model is required")
	}
	if embeddingModel == "" {
		return Settings{}, errors.New("embedding model is required")
	}

	settings := Settings{
		Version:            CurrentSettingsVersion,
		ConcurrentRequests: DefaultConcurrentRequests,
		CompletionModels: map[string]ModelConfig{
			"default_completion_model": {
				Provider: "openai", Model: completionModel, AuthMethod: "api_key",
				APIKey: "${MEKING_API_KEY}", StructuredOutput: StructuredOutputJSONSchema,
			},
		},
		EmbeddingModels: map[string]ModelConfig{
			"default_embedding_model": {
				Provider: "openai", Model: embeddingModel, AuthMethod: "api_key",
				APIKey: "${MEKING_API_KEY}",
			},
		},
		Input:    InputConfig{Type: InputTypeText, BaseDir: "input"},
		Chunking: ChunkingConfig{Type: "tokens", Size: 1200, Overlap: 100, EncodingModel: "o200k_base"},
		Storage:  StorageConfig{CacheDir: "cache"},
		Index:    defaultIndexSettings(),
		Query: QueryConfig{
			CompletionModelID:           "default_completion_model",
			GlobalSearchMapPrompt:       "prompts/global_search_map_system_prompt.txt",
			GlobalSearchReducePrompt:    "prompts/global_search_reduce_system_prompt.txt",
			GlobalSearchKnowledgePrompt: "prompts/global_search_knowledge_system_prompt.txt",
			DriftSearchPrompt:           "prompts/drift_search_system_prompt.txt",
			DriftReducePrompt:           "prompts/drift_reduce_prompt.txt",
			Global:                      defaultGlobalSearchConfig(),
			Drift:                       defaultDriftSearchConfig(),
		},
	}

	return settings, settings.Validate()
}

func (s Settings) Validate() error {
	if err := s.Activation.Validate(); err != nil {
		return err
	}
	if s.Version != CurrentSettingsVersion {
		return fmt.Errorf("unsupported settings version %d", s.Version)
	}
	if s.ConcurrentRequests <= 0 {
		return errors.New("concurrent requests must be positive")
	}
	if err := s.Index.validateStructure(); err != nil {
		return err
	}
	if _, ok := s.CompletionModels[s.Index.CompletionModelID]; !ok {
		return fmt.Errorf("unknown completion model id %q", s.Index.CompletionModelID)
	}
	if _, ok := s.EmbeddingModels[s.Index.EmbeddingModelID]; !ok {
		return fmt.Errorf("unknown embedding model id %q", s.Index.EmbeddingModelID)
	}
	if _, ok := s.CompletionModels[s.Query.CompletionModelID]; !ok {
		return fmt.Errorf("unknown query completion model id %q", s.Query.CompletionModelID)
	}
	for modelID, model := range s.CompletionModels {
		if err := validateCompletionModelConfig(model); err != nil {
			return fmt.Errorf("validate completion model %q: %w", modelID, err)
		}
	}
	for modelID, model := range s.EmbeddingModels {
		if err := validateEmbeddingModelConfig(model); err != nil {
			return fmt.Errorf("validate embedding model %q: %w", modelID, err)
		}
	}
	if err := validateProjectIndexPolicies(s); err != nil {
		return err
	}
	switch s.Chunking.Type {
	case "tokens":
		if s.Chunking.Size <= 0 {
			return errors.New("chunk size must be positive")
		}
		if s.Chunking.Overlap < 0 || s.Chunking.Overlap >= s.Chunking.Size {
			return errors.New("chunk overlap must be non-negative and smaller than size")
		}
	case "sentence":
		// Sentence uses NLTK boundaries and deliberately ignores token windows.
	default:
		return fmt.Errorf("unsupported chunking type %q", s.Chunking.Type)
	}
	if err := validateConfiguredRelativePath(s.Input.BaseDir, "input base directory"); err != nil {
		return err
	}
	if strings.TrimSpace(s.Input.FilePattern) != "" {
		if _, err := regexp.Compile(s.Input.FilePattern); err != nil {
			return fmt.Errorf("input file pattern is invalid: %w", err)
		}
	}
	switch s.Input.Type {
	case InputTypeText:
		if s.Input.MaxFileBytes != 0 {
			return errors.New("text input does not use max file bytes")
		}
	case InputTypeRichDocuments:
		if strings.TrimSpace(s.Input.Encoding) != "" {
			return errors.New("rich document input does not use text encoding")
		}
		if s.Input.MaxFileBytes < 0 || s.Input.MaxFileBytes > MaximumRichDocumentMaxBytes {
			return fmt.Errorf(
				"rich document max file bytes must be 0 for the default or between 1 and %d",
				MaximumRichDocumentMaxBytes,
			)
		}
	default:
		return fmt.Errorf("unsupported input type %q", s.Input.Type)
	}
	if err := validateConfiguredRelativePath(s.Storage.CacheDir, "cache directory"); err != nil {
		return err
	}
	for _, prompt := range []struct {
		label string
		path  string
	}{
		{label: "Global Map prompt", path: s.Query.GlobalSearchMapPrompt},
		{label: "Global Reduce prompt", path: s.Query.GlobalSearchReducePrompt},
		{label: "Global knowledge prompt", path: s.Query.GlobalSearchKnowledgePrompt},
		{label: "DRIFT search prompt", path: s.Query.DriftSearchPrompt},
		{label: "DRIFT Reduce prompt", path: s.Query.DriftReducePrompt},
	} {
		if err := validateConfiguredRelativePath(prompt.path, prompt.label); err != nil {
			return err
		}
	}
	if err := validateGlobalSearchConfig(s.Query.Global); err != nil {
		return fmt.Errorf("validate global search settings: %w", err)
	}
	if err := driftConfiguration(s.Query.Drift).Validate(); err != nil {
		return fmt.Errorf("validate DRIFT search settings: %w", err)
	}
	return nil
}

// RichDocumentMaxBytes resolves the bounded default for rich-document input.
func (config InputConfig) RichDocumentMaxBytes() int64 {
	if config.MaxFileBytes > 0 {
		return config.MaxFileBytes
	}
	return DefaultRichDocumentMaxBytes
}

func validateConfiguredRelativePath(value, label string) error {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return fmt.Errorf("%s must be a non-empty relative path", label)
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s must stay within its Project root", label)
	}
	return nil
}

// MarshalSettings validates and encodes settings as YAML.
func MarshalSettings(settings Settings) ([]byte, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal settings: %w", err)
	}
	return data, nil
}

// ParseSettings strictly decodes and validates YAML settings.
func ParseSettings(data []byte) (Settings, error) {
	var header struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return Settings{}, fmt.Errorf("decode settings version: %w", err)
	}
	if header.Version != CurrentSettingsVersion {
		return Settings{}, fmt.Errorf("unsupported settings version %d", header.Version)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var settings Settings
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	if err := applySettingsDefaults(data, &settings); err != nil {
		return Settings{}, err
	}
	if err := settings.Validate(); err != nil {
		return Settings{}, fmt.Errorf("validate settings: %w", err)
	}
	return settings, nil
}

func applySettingsDefaults(data []byte, settings *Settings) error {
	var presence struct {
		ConcurrentRequests *int `yaml:"concurrent_requests"`
		CompletionModels   map[string]struct {
			StructuredOutput *string `yaml:"structured_output"`
		} `yaml:"completion_models"`
		Index projectIndexFieldPresence `yaml:"index"`
		Query struct {
			DriftSearchPrompt *string `yaml:"drift_search_prompt"`
			DriftReducePrompt *string `yaml:"drift_reduce_prompt"`
			Global            *struct {
				MaxContextTokens        *int  `yaml:"max_context_tokens"`
				DataMaxTokens           *int  `yaml:"data_max_tokens"`
				MapMaxLength            *int  `yaml:"map_max_length"`
				ReduceMaxLength         *int  `yaml:"reduce_max_length"`
				DynamicSearchThreshold  *int  `yaml:"dynamic_search_threshold"`
				DynamicSearchKeepParent *bool `yaml:"dynamic_search_keep_parent"`
				DynamicSearchNumRepeats *int  `yaml:"dynamic_search_num_repeats"`
				DynamicSearchUseSummary *bool `yaml:"dynamic_search_use_summary"`
				DynamicSearchMaxLevel   *int  `yaml:"dynamic_search_max_level"`
			} `yaml:"global"`
			Drift *struct {
				CommunityLevel            *int    `yaml:"community_level"`
				Reports                   *int    `yaml:"reports"`
				PrimerFolds               *int    `yaml:"primer_folds"`
				BatchSize                 *int    `yaml:"batch_size"`
				FollowUpLimit             *int    `yaml:"follow_up_limit"`
				MaxDepth                  *int    `yaml:"max_depth"`
				MaxBranches               *int    `yaml:"max_branches"`
				MaxConcurrency            *int    `yaml:"max_concurrency"`
				MaxModelCalls             *int    `yaml:"max_model_calls"`
				MaxPromptTokens           *int    `yaml:"max_prompt_tokens"`
				MaxOutputTokens           *int    `yaml:"max_output_tokens"`
				MaxDurationSeconds        *int    `yaml:"max_duration_seconds"`
				PrimerMaxPromptTokens     *int    `yaml:"primer_max_prompt_tokens"`
				HyDEMaxCompletionTokens   *int    `yaml:"hyde_max_completion_tokens"`
				PrimerMaxCompletionTokens *int    `yaml:"primer_max_completion_tokens"`
				BranchMaxCompletionTokens *int    `yaml:"branch_max_completion_tokens"`
				ReduceMaxContextTokens    *int    `yaml:"reduce_max_context_tokens"`
				ReduceMaxCompletionTokens *int    `yaml:"reduce_max_completion_tokens"`
				ResponseType              *string `yaml:"response_type"`
			} `yaml:"drift"`
		} `yaml:"query"`
	}
	if err := yaml.Unmarshal(data, &presence); err != nil {
		return fmt.Errorf("inspect settings defaults: %w", err)
	}
	if presence.ConcurrentRequests == nil {
		settings.ConcurrentRequests = DefaultConcurrentRequests
	}
	for modelID, model := range settings.CompletionModels {
		if fields, found := presence.CompletionModels[modelID]; !found || fields.StructuredOutput == nil {
			model.StructuredOutput = StructuredOutputJSONSchema
			settings.CompletionModels[modelID] = model
		}
	}
	applyIndexSettingsDefaults(presence.Index, &settings.Index)
	if presence.Query.DriftSearchPrompt == nil {
		settings.Query.DriftSearchPrompt = "prompts/drift_search_system_prompt.txt"
	}
	if presence.Query.DriftReducePrompt == nil {
		settings.Query.DriftReducePrompt = "prompts/drift_reduce_prompt.txt"
	}
	globalDefaults := defaultGlobalSearchConfig()
	if presence.Query.Global == nil {
		settings.Query.Global = globalDefaults
	} else {
		if presence.Query.Global.MaxContextTokens == nil {
			settings.Query.Global.MaxContextTokens = globalDefaults.MaxContextTokens
		}
		if presence.Query.Global.DataMaxTokens == nil {
			settings.Query.Global.DataMaxTokens = globalDefaults.DataMaxTokens
		}
		if presence.Query.Global.MapMaxLength == nil {
			settings.Query.Global.MapMaxLength = globalDefaults.MapMaxLength
		}
		if presence.Query.Global.ReduceMaxLength == nil {
			settings.Query.Global.ReduceMaxLength = globalDefaults.ReduceMaxLength
		}
		if presence.Query.Global.DynamicSearchThreshold == nil {
			settings.Query.Global.DynamicSearchThreshold = globalDefaults.DynamicSearchThreshold
		}
		if presence.Query.Global.DynamicSearchKeepParent == nil {
			settings.Query.Global.DynamicSearchKeepParent = globalDefaults.DynamicSearchKeepParent
		}
		if presence.Query.Global.DynamicSearchNumRepeats == nil {
			settings.Query.Global.DynamicSearchNumRepeats = globalDefaults.DynamicSearchNumRepeats
		}
		if presence.Query.Global.DynamicSearchUseSummary == nil {
			settings.Query.Global.DynamicSearchUseSummary = globalDefaults.DynamicSearchUseSummary
		}
		if presence.Query.Global.DynamicSearchMaxLevel == nil {
			settings.Query.Global.DynamicSearchMaxLevel = globalDefaults.DynamicSearchMaxLevel
		}
	}
	driftDefaults := defaultDriftSearchConfig()
	if presence.Query.Drift == nil {
		settings.Query.Drift = driftDefaults
	} else {
		fields := presence.Query.Drift
		if fields.CommunityLevel == nil {
			settings.Query.Drift.CommunityLevel = driftDefaults.CommunityLevel
		}
		if fields.Reports == nil {
			settings.Query.Drift.Reports = driftDefaults.Reports
		}
		if fields.PrimerFolds == nil {
			settings.Query.Drift.PrimerFolds = driftDefaults.PrimerFolds
		}
		if fields.BatchSize == nil {
			settings.Query.Drift.BatchSize = driftDefaults.BatchSize
		}
		if fields.FollowUpLimit == nil {
			settings.Query.Drift.FollowUpLimit = driftDefaults.FollowUpLimit
		}
		if fields.MaxDepth == nil {
			settings.Query.Drift.MaxDepth = driftDefaults.MaxDepth
		}
		if fields.MaxBranches == nil {
			settings.Query.Drift.MaxBranches = driftDefaults.MaxBranches
		}
		if fields.MaxConcurrency == nil {
			settings.Query.Drift.MaxConcurrency = driftDefaults.MaxConcurrency
		}
		if fields.MaxModelCalls == nil {
			settings.Query.Drift.MaxModelCalls = driftDefaults.MaxModelCalls
		}
		if fields.MaxPromptTokens == nil {
			settings.Query.Drift.MaxPromptTokens = driftDefaults.MaxPromptTokens
		}
		if fields.MaxOutputTokens == nil {
			settings.Query.Drift.MaxOutputTokens = driftDefaults.MaxOutputTokens
		}
		if fields.MaxDurationSeconds == nil {
			settings.Query.Drift.MaxDurationSeconds = driftDefaults.MaxDurationSeconds
		}
		if fields.PrimerMaxPromptTokens == nil {
			settings.Query.Drift.PrimerMaxPromptTokens = driftDefaults.PrimerMaxPromptTokens
		}
		if fields.HyDEMaxCompletionTokens == nil {
			settings.Query.Drift.HyDEMaxCompletionTokens = driftDefaults.HyDEMaxCompletionTokens
		}
		if fields.PrimerMaxCompletionTokens == nil {
			settings.Query.Drift.PrimerMaxCompletionTokens = driftDefaults.PrimerMaxCompletionTokens
		}
		if fields.BranchMaxCompletionTokens == nil {
			settings.Query.Drift.BranchMaxCompletionTokens = driftDefaults.BranchMaxCompletionTokens
		}
		if fields.ReduceMaxContextTokens == nil {
			settings.Query.Drift.ReduceMaxContextTokens = driftDefaults.ReduceMaxContextTokens
		}
		if fields.ReduceMaxCompletionTokens == nil {
			settings.Query.Drift.ReduceMaxCompletionTokens = driftDefaults.ReduceMaxCompletionTokens
		}
		if fields.ResponseType == nil {
			settings.Query.Drift.ResponseType = driftDefaults.ResponseType
		}
	}
	return nil
}

func validateModelConfig(model ModelConfig) error {
	if baseURL := strings.TrimSpace(model.BaseURL); baseURL != "" {
		parsed, err := url.ParseRequestURI(baseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("model base URL must be an absolute HTTP or HTTPS URL")
		}
	}
	return nil
}

func validateCompletionModelConfig(model ModelConfig) error {
	if err := validateModelConfig(model); err != nil {
		return err
	}
	switch model.StructuredOutput {
	case StructuredOutputJSONSchema, StructuredOutputJSONObject:
		return nil
	default:
		return fmt.Errorf("unsupported structured output mode %q", model.StructuredOutput)
	}
}

func validateEmbeddingModelConfig(model ModelConfig) error {
	if err := validateModelConfig(model); err != nil {
		return err
	}
	if strings.TrimSpace(model.StructuredOutput) != "" {
		return errors.New("embedding model does not use structured output")
	}
	return nil
}

func validateSelectedOpenAIModel(capability string, config ModelConfig) error {
	if config.Provider != "openai" {
		return fmt.Errorf("unsupported %s model provider %q", capability, config.Provider)
	}
	if config.AuthMethod != "api_key" {
		return fmt.Errorf("unsupported %s auth method %q", capability, config.AuthMethod)
	}
	return nil
}

func defaultGlobalSearchConfig() GlobalSearchConfig {
	return GlobalSearchConfig{
		MaxContextTokens:        queryglobal.DefaultContextTokens,
		DataMaxTokens:           queryglobal.DefaultDataTokens,
		MapMaxLength:            queryglobal.DefaultMapLength,
		ReduceMaxLength:         queryglobal.DefaultReduceLength,
		DynamicSearchThreshold:  queryglobal.DefaultSelectionThreshold,
		DynamicSearchKeepParent: false,
		DynamicSearchNumRepeats: queryglobal.DefaultSelectionRepeats,
		DynamicSearchUseSummary: false,
		DynamicSearchMaxLevel:   queryglobal.DefaultSelectionMaxLevel,
	}
}

func defaultDriftSearchConfig() DriftSearchConfig {
	configuration := querydrift.DefaultConfiguration()
	return DriftSearchConfig{
		CommunityLevel: configuration.Primer.CommunityLevel,
		Reports:        configuration.Primer.Reports, PrimerFolds: configuration.Primer.Folds,
		BatchSize: configuration.Traversal.BatchSize, FollowUpLimit: configuration.Traversal.FollowUpLimit,
		MaxDepth: configuration.Traversal.MaxDepth, MaxBranches: configuration.Traversal.MaxBranches,
		MaxConcurrency:            configuration.Traversal.MaxConcurrency,
		MaxModelCalls:             configuration.Traversal.Limits.ModelCalls,
		MaxPromptTokens:           configuration.Traversal.Limits.PromptTokens,
		MaxOutputTokens:           configuration.Traversal.Limits.OutputTokens,
		MaxDurationSeconds:        int(configuration.Traversal.Limits.Duration / time.Second),
		PrimerMaxPromptTokens:     configuration.Primer.MaxPromptTokens,
		HyDEMaxCompletionTokens:   configuration.Primer.HyDEMaxCompletionTokens,
		PrimerMaxCompletionTokens: configuration.Primer.PrimerMaxCompletionTokens,
		BranchMaxCompletionTokens: configuration.Traversal.MaxCompletionTokens,
		ReduceMaxContextTokens:    configuration.Reduce.MaxContextTokens,
		ReduceMaxCompletionTokens: configuration.Reduce.MaxCompletionTokens,
		ResponseType:              configuration.Traversal.ResponseType,
	}
}

func driftConfiguration(config DriftSearchConfig) querydrift.Configuration {
	limits := querydrift.RequestLimits{
		ModelCalls: config.MaxModelCalls, PromptTokens: config.MaxPromptTokens,
		OutputTokens: config.MaxOutputTokens, Duration: time.Duration(config.MaxDurationSeconds) * time.Second,
	}
	return querydrift.Configuration{
		Primer: querydrift.PrimerConfig{
			CommunityLevel: config.CommunityLevel, Reports: config.Reports, Folds: config.PrimerFolds,
			MaxConcurrency:            config.MaxConcurrency,
			MaxPromptTokens:           config.PrimerMaxPromptTokens,
			HyDEMaxCompletionTokens:   config.HyDEMaxCompletionTokens,
			PrimerMaxCompletionTokens: config.PrimerMaxCompletionTokens,
		},
		Traversal: querydrift.TraversalConfig{
			MaxDepth: config.MaxDepth, BatchSize: config.BatchSize, FollowUpLimit: config.FollowUpLimit,
			MaxBranches: config.MaxBranches, MaxConcurrency: config.MaxConcurrency,
			MaxCompletionTokens: config.BranchMaxCompletionTokens,
			ResponseType:        config.ResponseType, Limits: limits,
		},
		Reduce: querydrift.ReduceConfig{
			MaxContextTokens:    config.ReduceMaxContextTokens,
			MaxCompletionTokens: config.ReduceMaxCompletionTokens,
			ResponseType:        config.ResponseType,
		},
	}
}

func validateGlobalSearchConfig(config GlobalSearchConfig) error {
	if config.MaxContextTokens <= 0 {
		return errors.New("global context token budget must be positive")
	}
	if config.DataMaxTokens <= 0 {
		return errors.New("global reduce data token budget must be positive")
	}
	if config.MapMaxLength <= 0 || config.ReduceMaxLength <= 0 {
		return errors.New("global response length limits must be positive")
	}
	if config.DynamicSearchThreshold < 0 {
		return errors.New("global dynamic search threshold must be non-negative")
	}
	if config.DynamicSearchNumRepeats <= 0 {
		return errors.New("global dynamic search repeats must be positive")
	}
	if config.DynamicSearchMaxLevel < 0 {
		return errors.New("global dynamic search max level must be non-negative")
	}
	return nil
}
