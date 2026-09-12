package report

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/memoria-space/meking/community"
	"golang.org/x/sync/errgroup"
)

// Generator creates a complete immutable Report collection from one validated
// CommunitySet and one fixed Knowledge read view. It never persists results or
// chooses which collection is visible to Query.
type Generator struct {
	model  community.ReportModel
	tokens community.ReportTokenCounter
	config Config
}

// NewGenerator constructs report generation from consumer-owned model and
// tokenizer ports. The dependencies must be safe for Config.MaxConcurrency.
func NewGenerator(
	model community.ReportModel,
	tokens community.ReportTokenCounter,
	config Config,
) (*Generator, error) {
	if model == nil {
		return nil, errors.New("community report model is required")
	}
	if tokens == nil {
		return nil, errors.New("community report token counter is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = 1
	}
	return &Generator{model: model, tokens: tokens, config: config}, nil
}

// Generate builds one immutable Report for every Community in the supplied
// CommunitySet. Levels run from deepest to root and one level uses bounded
// concurrency, but reports never consume child report output. Any failure
// rejects the complete result so callers cannot persist a partial hierarchy.
func (g *Generator) Generate(
	ctx context.Context,
	input Input,
) ([]Report, error) {
	if g == nil {
		return nil, errors.New("community report Generator is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contexts, err := buildReportContexts(input, g.tokens, g.config)
	if err != nil {
		return nil, err
	}
	settings := g.Settings()
	modelCalls := make(chan struct{}, g.config.MaxConcurrency)

	reports := make([]Report, 0, len(contexts))
	for start := 0; start < len(contexts); {
		end := start + 1
		for end < len(contexts) &&
			contexts[end].community.Level == contexts[start].community.Level {
			end++
		}
		levelReports, err := g.generateReportLevel(
			ctx,
			contexts[start:end],
			input.Period,
			settings,
			modelCalls,
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, levelReports...)
		start = end
	}
	return reports, nil
}

// Settings returns the exact immutable generation choices that Generate copies
// into every Report. Community derivation compares this value with published
// Reports so a model, Prompt, tokenizer, token budget, or length change can
// refresh Reports even when Knowledge content is unchanged.
func (g *Generator) Settings() Settings {
	if g == nil {
		return Settings{}
	}
	return settingsFromConfig(g.config)
}

func (g *Generator) generateReportLevel(
	ctx context.Context,
	contexts []reportContext,
	period string,
	settings Settings,
	modelCalls chan struct{},
) ([]Report, error) {
	reports := make([]Report, len(contexts))
	if g.config.MaxConcurrency == 1 {
		for index, current := range contexts {
			report, err := g.generateReport(
				ctx, current, period, settings, modelCalls,
			)
			if err != nil {
				return nil, err
			}
			reports[index] = report
		}
		return reports, nil
	}
	group, groupContext := errgroup.WithContext(ctx)
	for index := range contexts {
		index := index
		group.Go(func() error {
			var err error
			reports[index], err = g.generateReport(
				groupContext, contexts[index], period, settings, modelCalls,
			)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return reports, nil
}

func (g *Generator) generateReport(
	ctx context.Context,
	current reportContext,
	period string,
	settings Settings,
	modelCalls chan struct{},
) (Report, error) {
	draft, err := g.generateDraft(ctx, current, modelCalls)
	if err != nil {
		return Report{}, fmt.Errorf(
			"generate Report for Community %q: %w",
			current.community.ID,
			err,
		)
	}
	report, err := newReport(
		current.community.ID,
		period,
		draft,
		current.entities,
		current.relations,
		current.claims,
		current.textUnitIDs,
		settings,
	)
	if err != nil {
		return Report{}, fmt.Errorf(
			"finalize Report for Community %q: %w",
			current.community.ID,
			err,
		)
	}
	return report, nil
}

func (config Config) Validate() error {
	if config.MaxConcurrency < 0 {
		return errors.New("community report max concurrency must not be negative")
	}
	if strings.TrimSpace(config.Prompt) == "" {
		return errors.New("community Report prompt is required")
	}
	return validateReportSettings(settingsFromConfig(config))
}

func settingsFromConfig(config Config) Settings {
	return Settings{
		Model: config.Model,
		PromptHash: sha256.Sum256([]byte(
			config.Prompt + "\x00" + reportFragmentPrompt + "\x00" + reportFragmentMergePrompt,
		)),
		Tokenizer:       config.Tokenizer,
		MaxInputTokens:  config.MaxInputTokens,
		MaxReportLength: config.MaxReportLength,
	}
}
