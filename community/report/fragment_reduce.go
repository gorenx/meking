package report

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/memoria-space/meking/community"
	"golang.org/x/sync/errgroup"
)

func (g *Generator) generateDraft(
	ctx context.Context,
	current reportContext,
	modelCalls chan struct{},
) (community.ReportDraft, error) {
	if current.prompt != "" {
		return g.generateFinalDraft(ctx, current.prompt, modelCalls)
	}
	fragments, err := g.generateFragments(ctx, current.fragments, modelCalls)
	if err != nil {
		return community.ReportDraft{}, err
	}
	return g.reduceFragments(ctx, fragments, modelCalls)
}

func (g *Generator) generateFragments(
	ctx context.Context,
	prompts []string,
	modelCalls chan struct{},
) ([]string, error) {
	if len(prompts) == 0 {
		return nil, errors.New("over-budget Report requires evidence fragment prompts")
	}
	fragments := make([]string, len(prompts))
	if g.config.MaxConcurrency == 1 {
		for index, prompt := range prompts {
			fragment, err := g.generateFragment(ctx, prompt, modelCalls)
			if err != nil {
				return nil, fmt.Errorf("generate Report fragment %d: %w", index, err)
			}
			fragments[index] = fragment.Content
		}
		return fragments, nil
	}
	group, groupContext := errgroup.WithContext(ctx)
	for index, prompt := range prompts {
		index, prompt := index, prompt
		group.Go(func() error {
			fragment, err := g.generateFragment(groupContext, prompt, modelCalls)
			if err != nil {
				return fmt.Errorf("generate Report fragment %d: %w", index, err)
			}
			fragments[index] = fragment.Content
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return fragments, nil
}

func (g *Generator) reduceFragments(
	ctx context.Context,
	fragments []string,
	modelCalls chan struct{},
) (community.ReportDraft, error) {
	for {
		finalPrompt, fits, err := g.finalPromptForFragments(fragments)
		if err != nil {
			return community.ReportDraft{}, err
		}
		if fits {
			return g.generateFinalDraft(ctx, finalPrompt, modelCalls)
		}
		if len(fragments) == 1 {
			return community.ReportDraft{}, fmt.Errorf(
				"%w: one Report fragment cannot fit the final prompt within %d tokens",
				ErrReportInputTooLarge,
				g.config.MaxInputTokens,
			)
		}

		groups, reduced, err := g.widestFragmentGroups(fragments)
		if err != nil {
			return community.ReportDraft{}, err
		}
		if !reduced {
			return community.ReportDraft{}, fmt.Errorf(
				"%w: no pair of Report fragments fits an intermediate merge prompt within %d tokens",
				ErrReportInputTooLarge,
				g.config.MaxInputTokens,
			)
		}
		next := make([]string, len(groups))
		if g.config.MaxConcurrency == 1 {
			for index, values := range groups {
				if len(values) == 1 {
					next[index] = values[0]
					continue
				}
				prompt := renderFragmentMergePrompt(values, fragmentWordLimit(g.config))
				fragment, err := g.generateFragment(ctx, prompt, modelCalls)
				if err != nil {
					return community.ReportDraft{}, fmt.Errorf(
						"merge Report fragment group %d: %w", index, err,
					)
				}
				next[index] = fragment.Content
			}
			fragments = next
			continue
		}
		group, groupContext := errgroup.WithContext(ctx)
		for index, values := range groups {
			index, values := index, values
			if len(values) == 1 {
				next[index] = values[0]
				continue
			}
			group.Go(func() error {
				prompt := renderFragmentMergePrompt(values, fragmentWordLimit(g.config))
				fragment, err := g.generateFragment(groupContext, prompt, modelCalls)
				if err != nil {
					return fmt.Errorf("merge Report fragment group %d: %w", index, err)
				}
				next[index] = fragment.Content
				return nil
			})
		}
		if err := group.Wait(); err != nil {
			return community.ReportDraft{}, err
		}
		fragments = next
	}
}

func (g *Generator) finalPromptForFragments(fragments []string) (string, bool, error) {
	prompt, err := community.RenderReportPrompt(
		g.config.Prompt,
		renderFragments(fragments),
		g.config.MaxReportLength,
	)
	if err != nil {
		return "", false, fmt.Errorf("render final Report merge prompt: %w", err)
	}
	count, err := g.tokens.Count(prompt)
	if err != nil {
		return "", false, fmt.Errorf("count final Report merge prompt: %w", err)
	}
	if count < 0 {
		return "", false, errors.New("count final Report merge prompt: token counter returned a negative count")
	}
	return prompt, count <= g.config.MaxInputTokens, nil
}

func (g *Generator) widestFragmentGroups(fragments []string) ([][]string, bool, error) {
	groups := make([][]string, 0, len(fragments))
	reduced := false
	for start := 0; start < len(fragments); {
		end := start + 1
		for end < len(fragments) {
			candidate := fragments[start : end+1]
			prompt := renderFragmentMergePrompt(candidate, fragmentWordLimit(g.config))
			count, err := g.tokens.Count(prompt)
			if err != nil {
				return nil, false, fmt.Errorf("count intermediate Report merge prompt: %w", err)
			}
			if count < 0 {
				return nil, false, errors.New("count intermediate Report merge prompt: token counter returned a negative count")
			}
			if count > g.config.MaxInputTokens {
				break
			}
			end++
		}
		group := append([]string(nil), fragments[start:end]...)
		if len(group) > 1 {
			reduced = true
		}
		groups = append(groups, group)
		start = end
	}
	return groups, reduced, nil
}

func (g *Generator) generateFinalDraft(
	ctx context.Context,
	prompt string,
	modelCalls chan struct{},
) (community.ReportDraft, error) {
	if err := acquireModelCall(ctx, modelCalls); err != nil {
		return community.ReportDraft{}, err
	}
	defer releaseModelCall(modelCalls)
	return g.model.GenerateCommunityReport(ctx, community.ReportModelRequest{Prompt: prompt})
}

func (g *Generator) generateFragment(
	ctx context.Context,
	prompt string,
	modelCalls chan struct{},
) (community.ReportFragment, error) {
	if err := acquireModelCall(ctx, modelCalls); err != nil {
		return community.ReportFragment{}, err
	}
	defer releaseModelCall(modelCalls)
	fragment, err := g.model.GenerateReportFragment(
		ctx,
		community.ReportFragmentRequest{Prompt: prompt},
	)
	if err != nil {
		return community.ReportFragment{}, err
	}
	if strings.TrimSpace(fragment.Content) == "" {
		return community.ReportFragment{}, errors.New("Report model returned an empty Fragment")
	}
	return fragment, nil
}

func acquireModelCall(ctx context.Context, modelCalls chan struct{}) error {
	select {
	case modelCalls <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseModelCall(modelCalls chan struct{}) {
	<-modelCalls
}
