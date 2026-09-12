package extraction

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"
)

const (
	graphResultInstruction   = "Return only one JSON object matching the response schema supplied with this request. The schema overrides any legacy output-format instructions in the Project prompt."
	continueExtractionPrompt = "MANY entities and relations were missed in the last extraction. Return another JSON object containing only the missed items. Remember to ONLY emit entities that match any of the previously extracted types."
	loopExtractionPrompt     = "It appears some entities and relationships may have still been missed. Answer Y if there are still entities or relationships that need to be added, or N if there are none. Please answer with a single letter Y or N.\n"
	// DefaultGraphExtractionMaxGleanings performs one continuation after the
	// initial extraction response.
	DefaultGraphExtractionMaxGleanings = 1
)

var defaultGraphExtractionEntityTypes = [...]string{"organization", "person", "geo", "event"}

// GraphExtractionConfig controls prompt rendering and continuation attempts.
type GraphExtractionConfig struct {
	// Prompt is a format template containing entity_types and input_text fields.
	Prompt string
	// EntityTypes limits the categories requested from the completion model.
	EntityTypes []string
	// MaxGleanings is the maximum number of additional extraction rounds.
	MaxGleanings int
	// MaxConcurrency limits text units processed at the same time. Zero uses one.
	MaxConcurrency int
}

// DefaultGraphExtractionConfig returns an independent copy of the default
// knowledge-extraction policy. Prompt content and concurrency are supplied
// by the outer composition boundary.
func DefaultGraphExtractionConfig() GraphExtractionConfig {
	return GraphExtractionConfig{
		EntityTypes:  append([]string(nil), defaultGraphExtractionEntityTypes[:]...),
		MaxGleanings: DefaultGraphExtractionMaxGleanings,
	}
}

// Validate checks graph-extraction decisions without requiring a model or
// prompt resource. Prompt template validation remains a separate operation
// after its Project path has been resolved to content.
func (c GraphExtractionConfig) Validate() error {
	if c.MaxGleanings < 0 {
		return errors.New("graph extraction max gleanings must not be negative")
	}
	if c.MaxConcurrency < 0 {
		return errors.New("graph extraction max concurrency must not be negative")
	}
	return nil
}

// graphExtractor converts text units into source-linked graph observations.
type graphExtractor struct {
	// model supplies provider-neutral conversational text completion.
	model CompletionModel
	// config is an isolated copy of the extraction policy.
	config GraphExtractionConfig
}

// newGraphExtractor creates an extractor around the knowledge-owned completion port.
func newGraphExtractor(model CompletionModel, config GraphExtractionConfig) (*graphExtractor, error) {
	if model == nil {
		return nil, errors.New("completion model is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = 1
	}
	config.EntityTypes = append([]string(nil), config.EntityTypes...)
	return &graphExtractor{model: model, config: config}, nil
}

// extractGraph processes text units with bounded concurrency, restores input
// order, and rejects the complete batch when any TextUnit fails. Failures are
// reported in input order so concurrent provider completion cannot reorder diagnostics.
func (e *graphExtractor) extractGraph(ctx context.Context, units []TextUnitInput) (graphObservations, error) {
	type unitResult struct {
		observations graphObservations
		err          error
	}
	results := make([]unitResult, len(units))
	var group errgroup.Group
	group.SetLimit(e.config.MaxConcurrency)
	for index := range units {
		if err := ctx.Err(); err != nil {
			return graphObservations{}, err
		}
		idx := index
		group.Go(func() error {
			if err := ctx.Err(); err != nil {
				results[idx].err = err
				return nil
			}
			conversation, err := e.processDocument(ctx, strings.TrimSpace(units[idx].Text))
			if err != nil {
				results[idx].err = err
				return nil
			}
			observations, err := parseGraphExtraction(conversation.result, units[idx].ID)
			if err != nil {
				var rejection *rejectedResult
				if errors.As(err, &rejection) {
					conversation, err = e.graphDialogue().correct(ctx, e.model, conversation, rejection)
					if err == nil {
						observations, err = parseGraphExtraction(conversation.result, units[idx].ID)
					}
				}
			}
			if err != nil {
				results[idx].err = fmt.Errorf("parse graph extraction: %w", err)
				return nil
			}
			results[idx].observations = observations
			return nil
		})
	}
	_ = group.Wait()
	if err := ctx.Err(); err != nil {
		return graphObservations{}, err
	}

	all := emptyGraphObservations()
	failures := make([]error, 0)
	for index, result := range results {
		if result.err != nil {
			if isContextError(ctx, result.err) {
				return graphObservations{}, contextError(ctx, result.err)
			}
			failures = append(failures, fmt.Errorf(
				"extract graph from TextUnit %q: %w",
				units[index].ID,
				result.err,
			))
			continue
		}
		all.Entities = append(all.Entities, result.observations.Entities...)
		all.Relationships = append(all.Relationships, result.observations.Relationships...)
	}
	if len(failures) != 0 {
		return graphObservations{}, errors.Join(failures...)
	}

	if len(all.Entities) == 0 {
		return graphObservations{}, ErrNoEntitiesDetected
	}
	if err := validateRelationshipReferences(all.Relationships, all.Entities); err != nil {
		return graphObservations{}, err
	}
	return all, nil
}

func (e *graphExtractor) processDocument(ctx context.Context, text string) (extractionConversation, error) {
	prompt, err := renderPrompt(e.config.Prompt, map[string]string{
		"entity_types": strings.Join(e.config.EntityTypes, ","),
		"input_text":   text,
	})
	if err != nil {
		return extractionConversation{}, fmt.Errorf("render graph extraction prompt: %w", err)
	}
	prompt += "\n\n" + graphResultInstruction
	return e.graphDialogue().extract(ctx, e.model, prompt)
}

func (e *graphExtractor) graphDialogue() gleaningDialogue {
	return gleaningDialogue{
		stage:              "graph extraction",
		continuationPrompt: continueExtractionPrompt,
		loopPrompt:         loopExtractionPrompt,
		recordDelimiter:    "\n",
		maxGleanings:       e.config.MaxGleanings,
		invalidResponse:    ErrInvalidGraphExtraction,
		schemaName:         graphExtractionSchemaName,
		schema:             graphExtractionSchema,
	}
}

func validateRelationshipReferences(relationships []relationshipObservation, entities []entityObservation) error {
	titles := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		titles[entity.Title] = struct{}{}
	}
	for _, relationship := range relationships {
		_, sourceExists := titles[relationship.Source]
		_, targetExists := titles[relationship.Target]
		if !sourceExists || !targetExists {
			return fmt.Errorf(
				"%w: Relation %q -> %q",
				ErrUnresolvedEntityReference,
				relationship.Source,
				relationship.Target,
			)
		}
	}
	return nil
}

// validateGraphExtractionPrompt checks the template grammar and supported
// fields without invoking the completion model. Project preflight uses it
// before a run creates state or spends provider quota.
func validateGraphExtractionPrompt(template string) error {
	_, err := renderPrompt(template, map[string]string{
		"entity_types": "ENTITY",
		"input_text":   "input",
	})
	return err
}

func isContextError(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func contextError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
