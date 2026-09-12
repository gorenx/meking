package extraction

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"
)

const (
	continueClaimExtractionPrompt = "MANY entities were missed in the last extraction.  Add them below using the same format:\n"
	loopClaimExtractionPrompt     = "It appears some entities may have still been missed. Answer Y if there are still entities that need to be added, or N if there are none. Please answer with a single letter Y or N.\n"
	// DefaultClaimDescription is the broad product policy used when Claim
	// extraction is enabled without a narrower Project description.
	DefaultClaimDescription = "Any claims or facts that could be relevant to information discovery."
	// DefaultClaimMaxGleanings performs one continuation after the initial
	// Claim extraction response.
	DefaultClaimMaxGleanings = 1
)

// ClaimExtractionConfig controls which assertions are requested and how model
// calls are bounded without owning provider or cache policy.
type ClaimExtractionConfig struct {
	Prompt         string
	EntityTypes    []string
	Description    string
	MaxGleanings   int
	MaxConcurrency int
}

// DefaultClaimExtractionConfig returns the knowledge-owned Claim policy.
// Prompt content, entity types, and concurrency are supplied by composition.
func DefaultClaimExtractionConfig() ClaimExtractionConfig {
	return ClaimExtractionConfig{
		Description:  DefaultClaimDescription,
		MaxGleanings: DefaultClaimMaxGleanings,
	}
}

// Validate checks Claim extraction bounds independently of transport and model
// dependencies. An empty Description remains valid to the reusable domain
// service; a Project may impose a stricter product requirement.
func (c ClaimExtractionConfig) Validate() error {
	if c.MaxGleanings < 0 {
		return errors.New("claim extraction max gleanings must not be negative")
	}
	if c.MaxConcurrency < 0 {
		return errors.New("claim extraction max concurrency must not be negative")
	}
	return nil
}

// claimExtractor turns TextUnits into independently citable entity assertions.
// A failure for one TextUnit is reported and rejects the complete batch.
type claimExtractor struct {
	model  CompletionModel
	config ClaimExtractionConfig
}

// newclaimExtractor creates the Claim extraction use case around the
// knowledge-owned completion port.
func newclaimExtractor(
	model CompletionModel,
	config ClaimExtractionConfig,
) (*claimExtractor, error) {
	if model == nil {
		return nil, errors.New("claim completion model is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = 1
	}
	config.EntityTypes = append([]string(nil), config.EntityTypes...)
	return &claimExtractor{
		model: model, config: config,
	}, nil
}

// extractClaims processes TextUnits with bounded concurrency and restores source
// order. It does not allocate identities; MVCC reconciliation owns stable Claim
// identity after resolving the Subject.
func (e *claimExtractor) extractClaims(ctx context.Context, units []TextUnitInput) ([]extractedClaim, error) {
	type unitResult struct {
		claims []claimObservation
		err    error
	}
	results := make([]unitResult, len(units))
	var group errgroup.Group
	group.SetLimit(e.config.MaxConcurrency)
	for index := range units {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		idex := index
		group.Go(func() error {
			conversation, err := e.processTextUnit(ctx, strings.TrimSpace(units[idex].Text))
			if err != nil {
				results[idex].err = err
				return nil
			}
			claims, err := parseClaimExtraction(conversation.result, units[idex].ID)
			if err != nil {
				var rejection *rejectedResult
				if errors.As(err, &rejection) {
					conversation, err = e.claimDialogue().correct(ctx, e.model, conversation, rejection)
					if err == nil {
						claims, err = parseClaimExtraction(conversation.result, units[idex].ID)
					}
				}
			}
			if err != nil {
				results[idex].err = fmt.Errorf("parse Claim extraction: %w", err)
				return nil
			}
			results[idex].claims = claims
			return nil
		})
	}
	_ = group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	observations := make([]claimObservation, 0)
	failures := make([]error, 0)
	for index, result := range results {
		if result.err != nil {
			if isContextError(ctx, result.err) {
				return nil, contextError(ctx, result.err)
			}
			failures = append(failures, fmt.Errorf(
				"extract Claims from TextUnit %q: %w",
				units[index].ID,
				result.err,
			))
			continue
		}
		observations = append(observations, result.claims...)
	}
	if len(failures) != 0 {
		return nil, errors.Join(failures...)
	}

	claims := make([]extractedClaim, 0, len(observations))
	for _, observation := range observations {
		claims = append(claims, extractedClaim{
			Subject: observation.Subject, Object: observation.Object,
			Type: observation.Type, Status: observation.Status,
			StartDate: observation.StartDate, EndDate: observation.EndDate,
			Description: observation.Description, SourceText: observation.SourceText,
			TextUnitID: observation.TextUnitID,
		})
	}
	return claims, nil
}

func (e *claimExtractor) processTextUnit(ctx context.Context, text string) (extractionConversation, error) {
	prompt, err := renderPrompt(e.config.Prompt, map[string]string{
		"input_text":        text,
		"entity_specs":      formatClaimEntitySpecs(e.config.EntityTypes),
		"claim_description": e.config.Description,
	})
	if err != nil {
		return extractionConversation{}, fmt.Errorf("render claim extraction prompt: %w", err)
	}
	return e.claimDialogue().extract(ctx, e.model, prompt)
}

func (e *claimExtractor) claimDialogue() gleaningDialogue {
	return gleaningDialogue{
		stage:               "claim extraction",
		continuationPrompt:  continueClaimExtractionPrompt,
		loopPrompt:          loopClaimExtractionPrompt,
		completionDelimiter: claimCompletionDelimiter,
		recordDelimiter:     claimRecordDelimiter,
		maxGleanings:        e.config.MaxGleanings,
		invalidResponse:     ErrInvalidClaimExtraction,
	}
}

// validateClaimExtractionPrompt checks the template grammar and supported
// fields without invoking the completion model. It is safe to call during a
// read-only index preflight.
func validateClaimExtractionPrompt(template string) error {
	_, err := renderPrompt(template, map[string]string{
		"input_text":        "input",
		"entity_specs":      "['ENTITY']",
		"claim_description": "description",
	})
	return err
}

func formatClaimEntitySpecs(entityTypes []string) string {
	quoted := make([]string, len(entityTypes))
	for index, entityType := range entityTypes {
		quoted[index] = "'" + strings.ReplaceAll(entityType, "'", "\\'") + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
