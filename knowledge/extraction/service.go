package extraction

import (
	"errors"
	"fmt"
	"strings"
)

// Policy contains the output-affecting extraction decisions for one command.
type Policy struct {
	Graph  GraphExtractionConfig
	Claims *ClaimExtractionConfig
}

// Validate rejects an incomplete extraction policy before model work starts.
func (p Policy) Validate() error {
	if err := p.Graph.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(p.Graph.Prompt) == "" {
		return errors.New("graph extraction prompt is required")
	}
	if err := validateGraphExtractionPrompt(p.Graph.Prompt); err != nil {
		return fmt.Errorf("validate graph extraction prompt: %w", err)
	}
	if p.Claims == nil {
		return nil
	}
	if err := p.Claims.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(p.Claims.Prompt) == "" {
		return errors.New("Claim extraction prompt is required")
	}
	if err := validateClaimExtractionPrompt(p.Claims.Prompt); err != nil {
		return fmt.Errorf("validate Claim extraction prompt: %w", err)
	}
	return nil
}

// Model contains the validated model-facing dependencies shared by all
// extraction stages.
type Model struct {
	completion     CompletionModel
	maxConcurrency int
}

func NewModel(completion CompletionModel, maxConcurrency int) (Model, error) {
	switch {
	case completion == nil:
		return Model{}, errors.New("extraction completion model is required")
	case maxConcurrency <= 0:
		return Model{}, errors.New("extraction maximum concurrency must be positive")
	default:
		return Model{
			completion:     completion,
			maxConcurrency: maxConcurrency,
		}, nil
	}
}

// Service implements the model stages advanced by the recoverable Extraction
// Application.
type Service struct {
	model  Model
	policy Policy
}

func NewService(model Model, policy Policy) (*Service, error) {
	if model.completion == nil || model.maxConcurrency <= 0 {
		return nil, errors.New("extraction model is incomplete")
	}
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("validate extraction policy: %w", err)
	}
	return &Service{
		model:  model,
		policy: policy,
	}, nil
}

func validateTextUnits(units []TextUnitInput) error {
	if len(units) == 0 {
		return errors.New("extraction requires at least one TextUnit")
	}
	textsByID := make(map[string]string, len(units))
	for index, unit := range units {
		if strings.TrimSpace(unit.ID) == "" {
			return fmt.Errorf("TextUnit %d has an empty ID", index)
		}
		if strings.TrimSpace(unit.Text) == "" {
			return fmt.Errorf("TextUnit %q has empty text", unit.ID)
		}
		if previous, exists := textsByID[unit.ID]; exists && previous != unit.Text {
			return fmt.Errorf("TextUnit ID %q identifies different text", unit.ID)
		}
		textsByID[unit.ID] = unit.Text
	}
	return nil
}
