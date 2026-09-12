package drift

import (
	"errors"
	"strings"
	"time"
)

const (
	// DefaultPrimerReports matches the established DRIFT global-retrieval width.
	DefaultPrimerReports = 20
	// DefaultPrimerFolds bounds how many independent report groups are analyzed.
	DefaultPrimerFolds = 5
	// DefaultPrimerConcurrency bounds simultaneous structured Primer calls.
	DefaultPrimerConcurrency = 8
	// DefaultPrimerCommunityLevel includes Reports at or below this hierarchy level.
	DefaultPrimerCommunityLevel = 2
	// DefaultPrimerPromptTokens is the hard token limit for each model prompt.
	DefaultPrimerPromptTokens = 12000
	// DefaultHyDECompletionTokens caps the hypothetical retrieval answer.
	DefaultHyDECompletionTokens = 2_000
	// DefaultPrimerCompletionTokens caps each structured fold response.
	DefaultPrimerCompletionTokens = 2_000

	DefaultTraversalDepth            = 3
	DefaultTraversalBatchSize        = 20
	DefaultTraversalFollowUpLimit    = 20
	DefaultTraversalBranches         = 60
	DefaultTraversalConcurrency      = 8
	DefaultTraversalCompletionTokens = 2_000
	DefaultTraversalResponseType     = "multiple paragraphs"
	DefaultRequestModelCalls         = 80
	DefaultRequestPromptTokens       = 1_000_000
	DefaultRequestOutputTokens       = 160_000
	DefaultRequestDuration           = 5 * time.Minute
	DefaultReduceContextTokens       = 12_000
	DefaultReduceCompletionTokens    = 2_000
	DefaultReduceResponseType        = "multiple paragraphs"
)

// PrimerConfig bounds individual calls in the initial report retrieval and
// decomposition stage. Searcher separately enforces cumulative RequestLimits.
type PrimerConfig struct {
	// CommunityLevel includes Reports whose Community level is less than or equal
	// to this non-negative value.
	CommunityLevel int
	// Reports is the positive maximum number of cosine-ranked Reports selected
	// from the Epoch's ReportSet.
	Reports int
	// Folds is the positive requested number of deterministic contiguous Report
	// groups. Empty groups are not created when fewer Reports are selected.
	Folds int
	// MaxConcurrency is the positive maximum number of folds analyzed at once.
	MaxConcurrency int
	// MaxPromptTokens is the positive per-call limit measured with the Epoch's
	// Corpora tokenizer before either model is invoked.
	MaxPromptTokens int
	// HyDEMaxCompletionTokens bounds the hypothetical retrieval answer.
	HyDEMaxCompletionTokens int
	// PrimerMaxCompletionTokens bounds each structured fold response.
	PrimerMaxCompletionTokens int
}

// DefaultPrimerConfig returns the bounded initial DRIFT policy.
func DefaultPrimerConfig() PrimerConfig {
	return PrimerConfig{
		CommunityLevel:            DefaultPrimerCommunityLevel,
		Reports:                   DefaultPrimerReports,
		Folds:                     DefaultPrimerFolds,
		MaxConcurrency:            DefaultPrimerConcurrency,
		MaxPromptTokens:           DefaultPrimerPromptTokens,
		HyDEMaxCompletionTokens:   DefaultHyDECompletionTokens,
		PrimerMaxCompletionTokens: DefaultPrimerCompletionTokens,
	}
}

// Validate rejects policies that cannot bound retrieval or model work.
func (c PrimerConfig) Validate() error {
	switch {
	case c.CommunityLevel < 0:
		return errors.New("DRIFT Primer Community level must be non-negative")
	case c.Reports <= 0:
		return errors.New("DRIFT Primer report limit must be positive")
	case c.Folds <= 0:
		return errors.New("DRIFT Primer fold count must be positive")
	case c.MaxConcurrency <= 0:
		return errors.New("DRIFT Primer concurrency must be positive")
	case c.MaxPromptTokens <= 0:
		return errors.New("DRIFT Primer prompt token limit must be positive")
	case c.HyDEMaxCompletionTokens <= 0:
		return errors.New("DRIFT HyDE completion token limit must be positive")
	case c.PrimerMaxCompletionTokens <= 0:
		return errors.New("DRIFT Primer fold completion token limit must be positive")
	default:
		return nil
	}
}

// RequestLimits are cumulative across Primer, Local traversal, and final
// Reduce. A stage must reserve its next model call before dispatching it.
type RequestLimits struct {
	ModelCalls   int
	PromptTokens int
	OutputTokens int
	Duration     time.Duration
}

func DefaultRequestLimits() RequestLimits {
	return RequestLimits{
		ModelCalls: DefaultRequestModelCalls, PromptTokens: DefaultRequestPromptTokens,
		OutputTokens: DefaultRequestOutputTokens, Duration: DefaultRequestDuration,
	}
}

func (c RequestLimits) Validate() error {
	switch {
	case c.ModelCalls <= 0:
		return errors.New("DRIFT request model call limit must be positive")
	case c.PromptTokens <= 0:
		return errors.New("DRIFT request prompt token limit must be positive")
	case c.OutputTokens <= 0:
		return errors.New("DRIFT request output token limit must be positive")
	case c.Duration <= 0:
		return errors.New("DRIFT request duration must be positive")
	default:
		return nil
	}
}

// TraversalConfig bounds the Local branch graph independently of Local's own
// retrieval limits while carrying the request-wide limits shared with Reduce.
type TraversalConfig struct {
	MaxDepth            int
	BatchSize           int
	FollowUpLimit       int
	MaxBranches         int
	MaxConcurrency      int
	MaxCompletionTokens int
	ResponseType        string
	Limits              RequestLimits
}

// DefaultTraversalConfig returns the bounded DRIFT Local policy.
func DefaultTraversalConfig() TraversalConfig {
	return TraversalConfig{
		MaxDepth:            DefaultTraversalDepth,
		BatchSize:           DefaultTraversalBatchSize,
		FollowUpLimit:       DefaultTraversalFollowUpLimit,
		MaxBranches:         DefaultTraversalBranches,
		MaxConcurrency:      DefaultTraversalConcurrency,
		ResponseType:        DefaultTraversalResponseType,
		MaxCompletionTokens: DefaultTraversalCompletionTokens,
		Limits:              DefaultRequestLimits(),
	}
}

// Validate rejects policies that cannot put a finite bound on traversal work.
func (c TraversalConfig) Validate() error {
	switch {
	case c.MaxDepth <= 0:
		return errors.New("DRIFT traversal depth must be positive")
	case c.BatchSize <= 0:
		return errors.New("DRIFT traversal batch size must be positive")
	case c.FollowUpLimit <= 0:
		return errors.New("DRIFT traversal follow-up limit must be positive")
	case c.MaxBranches <= 0:
		return errors.New("DRIFT traversal branch limit must be positive")
	case c.MaxConcurrency <= 0:
		return errors.New("DRIFT traversal concurrency must be positive")
	case c.MaxCompletionTokens <= 0:
		return errors.New("DRIFT traversal completion token limit must be positive")
	case strings.TrimSpace(c.ResponseType) == "" || c.ResponseType != strings.TrimSpace(c.ResponseType):
		return errors.New("DRIFT traversal response type is required without surrounding whitespace")
	default:
		return c.Limits.Validate()
	}
}

// ReduceConfig bounds the complete intermediate-answer context and final model
// output independently of the request-wide cumulative limits.
type ReduceConfig struct {
	MaxContextTokens    int
	MaxCompletionTokens int
	ResponseType        string
}

// Configuration contains the complete immutable policy for one DRIFT Searcher.
type Configuration struct {
	Primer    PrimerConfig
	Traversal TraversalConfig
	Reduce    ReduceConfig
}

func DefaultConfiguration() Configuration {
	return Configuration{
		Primer: DefaultPrimerConfig(), Traversal: DefaultTraversalConfig(), Reduce: DefaultReduceConfig(),
	}
}

func (c Configuration) Validate() error {
	if err := c.Primer.Validate(); err != nil {
		return err
	}
	if err := c.Traversal.Validate(); err != nil {
		return err
	}
	if err := c.Reduce.Validate(); err != nil {
		return err
	}
	return validateMandatoryRequestLimits(c.Primer, c.Reduce, c.Traversal.Limits)
}

func DefaultReduceConfig() ReduceConfig {
	return ReduceConfig{
		MaxContextTokens: DefaultReduceContextTokens, MaxCompletionTokens: DefaultReduceCompletionTokens,
		ResponseType: DefaultReduceResponseType,
	}
}

func (c ReduceConfig) Validate() error {
	switch {
	case c.MaxContextTokens <= 0:
		return errors.New("DRIFT Reduce context token limit must be positive")
	case c.MaxCompletionTokens <= 0:
		return errors.New("DRIFT Reduce completion token limit must be positive")
	case strings.TrimSpace(c.ResponseType) == "" || c.ResponseType != strings.TrimSpace(c.ResponseType):
		return errors.New("DRIFT Reduce response type is required without surrounding whitespace")
	default:
		return nil
	}
}

func validateMandatoryRequestLimits(primer PrimerConfig, reduce ReduceConfig, limits RequestLimits) error {
	folds := min(primer.Reports, primer.Folds)
	primerCalls := 1 + folds
	switch {
	case primerCalls+1 > limits.ModelCalls:
		return errors.New("DRIFT request model call limit cannot contain Primer and Reduce")
	case exceedsProduct(primer.MaxPromptTokens, primerCalls, limits.PromptTokens):
		return errors.New("DRIFT request prompt token limit cannot contain Primer")
	case primer.HyDEMaxCompletionTokens+folds*primer.PrimerMaxCompletionTokens+reduce.MaxCompletionTokens > limits.OutputTokens:
		return errors.New("DRIFT request output token limit cannot contain Primer and Reduce")
	default:
		return nil
	}
}

func exceedsProduct(perCall int, calls int, limit int) bool {
	return calls > 0 && perCall > limit/calls
}
