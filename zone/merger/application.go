package merger

import (
	"context"
	"fmt"

	"github.com/memoria-space/meking/zone"
)

type ZoneResolver interface {
	ResolveDirectParent(ctx context.Context, childID zone.ID) (zone.ID, error)
}

type Corpus interface {
	ValidateChildCorpora(ctx context.Context, sourceCorporaID string) error
	MergeChildCorpora(ctx context.Context, sourceCorporaID string) error
}

type Knowledge interface {
	MergeChildKnowledge(context.Context) error
}

type Dependencies struct {
	Zones     ZoneResolver
	Corpus    Corpus
	Knowledge Knowledge
}

type Application struct {
	dependencies Dependencies
}

func New(dependencies Dependencies) (*Application, error) {
	switch {
	case dependencies.Zones == nil:
		return nil, fmt.Errorf("%w: Zones are required", ErrNotReady)
	case dependencies.Corpus == nil:
		return nil, fmt.Errorf("%w: Corpus is required", ErrNotReady)
	case dependencies.Knowledge == nil:
		return nil, fmt.Errorf("%w: Knowledge is required", ErrNotReady)
	default:
		return &Application{dependencies: dependencies}, nil
	}
}

func (application *Application) Merge(ctx context.Context, input Input) error {
	if application == nil {
		return ErrNotReady
	}
	if err := validateInput(input); err != nil {
		return err
	}
	parentZoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	childZoneID, err := zone.RequireChildID(ctx)
	if err != nil {
		return err
	}
	resolvedParent, err := application.dependencies.Zones.ResolveDirectParent(ctx, childZoneID)
	if err != nil {
		return fmt.Errorf("resolve Child Zone direct Parent: %w", err)
	}
	if resolvedParent != parentZoneID {
		return fmt.Errorf(
			"%w: Child %q belongs to Parent %q, current Zone is %q",
			ErrNotDirectParent,
			childZoneID,
			resolvedParent,
			parentZoneID,
		)
	}
	if err := application.dependencies.Corpus.ValidateChildCorpora(ctx, input.SourceCorporaID); err != nil {
		return fmt.Errorf("validate Child Corpus boundary: %w", err)
	}
	if err := application.dependencies.Corpus.MergeChildCorpora(ctx, input.SourceCorporaID); err != nil {
		return fmt.Errorf("submit Child Corpus boundary: %w", err)
	}
	if err := application.dependencies.Knowledge.MergeChildKnowledge(ctx); err != nil {
		return fmt.Errorf("submit Child Knowledge boundary: %w", err)
	}
	return nil
}

func validateInput(input Input) error {
	if _, err := parseCorporaID(input.SourceCorporaID); err != nil {
		return ErrInvalidInput
	}
	return nil
}
