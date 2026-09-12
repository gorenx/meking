package global

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
	"golang.org/x/sync/errgroup"
)

const (
	// DefaultSelectionThreshold accepts ratings of one or greater.
	DefaultSelectionThreshold = 1
	// DefaultSelectionRepeats rates each visited Report once.
	DefaultSelectionRepeats = 1
	// DefaultSelectionMaxLevel allows fallback through hierarchy level two.
	DefaultSelectionMaxLevel = 2
	// DefaultSelectionConcurrency bounds simultaneous rating calls.
	DefaultSelectionConcurrency = 32
)

// RatingRequest is the provider-independent two-message input for deciding
// whether one immutable Report can help answer the current question.
type RatingRequest struct {
	// SystemPrompt contains the selected Report text and question.
	SystemPrompt string
	// UserPrompt repeats the unmodified question as the user message.
	UserPrompt string
}

// RatingModel rates one Report as JSON text without owning hierarchy traversal.
type RatingModel interface {
	RateCommunity(ctx context.Context, request RatingRequest) (string, error)
}

// RatingCorrection is a rejected relevance rating returned to the same Agent.
type RatingCorrection struct {
	Request RatingRequest
	Result  string
	Reason  string
}

type RatingCorrector interface {
	CorrectRating(ctx context.Context, correction RatingCorrection) (string, error)
}

// DynamicSelectionConfig defines when relevance traversal keeps a Report,
// descends into children, or falls back to another hierarchy level.
type DynamicSelectionConfig struct {
	// Threshold accepts ratings greater than or equal to this value.
	Threshold int
	// KeepParent retains an accepted parent after an accepted child is found.
	KeepParent bool
	// Repeats is the positive number of sequential votes for each visited Report.
	Repeats int
	// UseSummary rates Summary instead of FullContent.
	UseSummary bool
	// MaxLevel is the highest hierarchy level used to seed fallback traversal.
	MaxLevel int
	// MaxConcurrency bounds Reports rated together at one traversal depth.
	MaxConcurrency int
}

// DefaultDynamicSelectionConfig returns the bounded hierarchy traversal policy.
func DefaultDynamicSelectionConfig() DynamicSelectionConfig {
	return DynamicSelectionConfig{
		Threshold: DefaultSelectionThreshold, Repeats: DefaultSelectionRepeats,
		MaxLevel: DefaultSelectionMaxLevel, MaxConcurrency: DefaultSelectionConcurrency,
	}
}

// Validate rejects selection policies that cannot make bounded model calls.
func (c DynamicSelectionConfig) Validate() error {
	if c.Threshold < 0 {
		return errors.New("Global dynamic selection threshold must be non-negative")
	}
	if c.Repeats <= 0 {
		return errors.New("Global dynamic selection repeats must be positive")
	}
	if c.MaxLevel < 0 {
		return errors.New("Global dynamic selection maximum level must be non-negative")
	}
	if c.MaxConcurrency <= 0 {
		return errors.New("Global dynamic selection concurrency must be positive")
	}
	return nil
}

// DynamicCommunitySelector rates deterministic hierarchy frontiers and
// returns the remaining relevant Community IDs in traversal order.
type DynamicCommunitySelector struct {
	// model supplies only Report relevance ratings.
	model RatingModel
	// prompt is rendered independently for every visited Report.
	prompt string
	// config fixes traversal, voting, and concurrency policy.
	config DynamicSelectionConfig
}

// NewDynamicCommunitySelector creates a provider-independent hierarchy selector.
func NewDynamicCommunitySelector(
	model RatingModel,
	prompt string,
	config DynamicSelectionConfig,
) (*DynamicCommunitySelector, error) {
	if model == nil {
		return nil, errors.New("create Global dynamic selector: RatingModel is required")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("create Global dynamic selector: rating prompt is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &DynamicCommunitySelector{model: model, prompt: prompt, config: config}, nil
}

// SelectCommunities starts at hierarchy level zero and descends through
// relevant branches. When one starting level has no relevant Report, the next
// complete hierarchy level is tried through MaxLevel.
func (s *DynamicCommunitySelector) SelectCommunities(
	ctx context.Context,
	request SelectionRequest,
) ([]string, error) {
	if s == nil || s.model == nil {
		return nil, errors.New("Global dynamic selector is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hierarchy := buildSelectionHierarchy(request.Reports)
	for level := 0; level <= s.config.MaxLevel; level++ {
		seeds := hierarchy.levels[level]
		if len(seeds) == 0 {
			continue
		}
		selected, err := s.traverse(ctx, request.Question, seeds, hierarchy)
		if err != nil {
			return nil, err
		}
		if len(selected) > 0 {
			return selected, nil
		}
	}
	return nil, nil
}

// selectionHierarchy derives children from the single ParentID fact and keeps
// every frontier in the deterministic ReportSet order supplied by the caller.
type selectionHierarchy struct {
	// reports resolves the exact immutable Report text by Community ID.
	reports map[string]ReportCandidate
	// children contains only candidates whose parent is also selectable.
	children map[string][]string
	// levels contains candidate Community IDs in their input order.
	levels map[int][]string
}

func buildSelectionHierarchy(candidates []ReportCandidate) selectionHierarchy {
	result := selectionHierarchy{
		reports:  make(map[string]ReportCandidate, len(candidates)),
		children: make(map[string][]string),
		levels:   make(map[int][]string),
	}
	for _, candidate := range candidates {
		id := candidate.Community.ID
		result.reports[id] = candidate
		result.levels[candidate.Community.Level] = append(
			result.levels[candidate.Community.Level],
			id,
		)
	}
	for _, candidate := range candidates {
		if candidate.Community.ParentID == nil {
			continue
		}
		parentID := *candidate.Community.ParentID
		result.children[parentID] = append(
			result.children[parentID],
			candidate.Community.ID,
		)
	}
	return result
}

func (s *DynamicCommunitySelector) traverse(
	ctx context.Context,
	question string,
	seeds []string,
	hierarchy selectionHierarchy,
) ([]string, error) {
	queue := append([]string(nil), seeds...)
	selected := make(map[string]struct{})
	order := make([]string, 0)
	for len(queue) > 0 {
		ratings, err := s.rateFrontier(ctx, question, queue, hierarchy.reports)
		if err != nil {
			return nil, err
		}
		next := make([]string, 0)
		for index, id := range queue {
			if ratings[index] < s.config.Threshold {
				continue
			}
			if _, found := selected[id]; !found {
				selected[id] = struct{}{}
				order = append(order, id)
			}
			candidate := hierarchy.reports[id]
			if !s.config.KeepParent && candidate.Community.ParentID != nil {
				delete(selected, *candidate.Community.ParentID)
			}
			next = append(next, hierarchy.children[id]...)
		}
		queue = next
	}
	result := make([]string, 0, len(selected))
	for _, id := range order {
		if _, found := selected[id]; found {
			result = append(result, id)
		}
	}
	return result, nil
}

func (s *DynamicCommunitySelector) rateFrontier(
	ctx context.Context,
	question string,
	communityIDs []string,
	reports map[string]ReportCandidate,
) ([]int, error) {
	ratings := make([]int, len(communityIDs))
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(s.config.MaxConcurrency)
	for index, communityID := range communityIDs {
		index, communityID := index, communityID
		group.Go(func() error {
			candidate := reports[communityID]
			description := candidate.Report.FullContent
			if s.config.UseSummary {
				description = candidate.Report.Summary
			}
			rating, err := s.rateReport(groupContext, question, description)
			if err != nil {
				return err
			}
			ratings[index] = rating
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return ratings, nil
}

func (s *DynamicCommunitySelector) rateReport(
	ctx context.Context,
	question string,
	description string,
) (int, error) {
	systemPrompt, err := queryprompt.Render(s.prompt, map[string]string{
		"description": description,
		"question":    question,
	})
	if err != nil {
		return 0, fmt.Errorf("render Global rating prompt: %w", err)
	}
	ratings := make([]int, 0, s.config.Repeats)
	for repeat := 0; repeat < s.config.Repeats; repeat++ {
		request := RatingRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   question,
		}
		response, err := s.model.RateCommunity(ctx, request)
		if err != nil {
			return 0, err
		}
		rating, err := parseRating(response)
		if err != nil {
			corrector, supported := s.model.(RatingCorrector)
			if !supported {
				return 0, querybase.NewInvalidModelResponseFailure(err)
			}
			response, err = corrector.CorrectRating(ctx, RatingCorrection{
				Request: request,
				Result:  response,
				Reason:  err.Error(),
			})
			if err != nil {
				return 0, err
			}
			rating, err = parseRating(response)
			if err != nil {
				return 0, querybase.NewInvalidModelResponseFailure(err)
			}
		}
		ratings = append(ratings, rating)
	}
	return majorityRating(ratings), nil
}

func parseRating(response string) (int, error) {
	object, err := decodeJSONObject(response)
	if err != nil {
		return 0, err
	}
	rating, found := object["rating"]
	if !found {
		return 0, errors.New("Global rating response is missing rating")
	}
	return parseInteger(rating)
}

func majorityRating(ratings []int) int {
	counts := make(map[int]int)
	options := make([]int, 0)
	for _, rating := range ratings {
		if _, found := counts[rating]; !found {
			options = append(options, rating)
		}
		counts[rating]++
	}
	sort.Ints(options)
	selected, maximum := 0, -1
	for _, rating := range options {
		if counts[rating] > maximum {
			selected = rating
			maximum = counts[rating]
		}
	}
	return selected
}
