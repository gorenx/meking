package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/controlplane"
	"github.com/memoria-space/meking/zone"
)

type AutomaticZones interface {
	List(context.Context) ([]zone.Definition, error)
}

type AutomaticActions interface {
	InvokeAutomatic(
		context.Context,
		controlplane.Action,
		time.Time,
	) (Invocation, bool, error)
}

type AutomaticWakeups interface {
	Wakeups() <-chan struct{}
}

type AutomaticEvaluator struct {
	zones        AutomaticZones
	actions      AutomaticActions
	wakeups      <-chan struct{}
	pollInterval time.Duration
}

func NewAutomaticEvaluator(
	zones AutomaticZones,
	actions AutomaticActions,
	wakeups AutomaticWakeups,
	pollInterval time.Duration,
) (*AutomaticEvaluator, error) {
	switch {
	case zones == nil:
		return nil, errors.New("create Automatic Evaluator: Zones are required")
	case actions == nil:
		return nil, errors.New("create Automatic Evaluator: Actions are required")
	case wakeups == nil:
		return nil, errors.New("create Automatic Evaluator: Wakeups are required")
	case pollInterval <= 0:
		return nil, errors.New("create Automatic Evaluator: PollInterval must be positive")
	}
	wakeupChannel := wakeups.Wakeups()
	if wakeupChannel == nil {
		return nil, errors.New("create Automatic Evaluator: Wakeup channel is required")
	}
	return &AutomaticEvaluator{
		zones:        zones,
		actions:      actions,
		wakeups:      wakeupChannel,
		pollInterval: pollInterval,
	}, nil
}

func (evaluator *AutomaticEvaluator) Run(ctx context.Context) error {
	if evaluator == nil {
		return errors.New("run Automatic Evaluator: Evaluator is required")
	}
	if err := evaluator.scan(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(evaluator.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-evaluator.wakeups:
			if err := evaluator.scan(ctx); err != nil {
				return err
			}
		case <-ticker.C:
			if err := evaluator.scan(ctx); err != nil {
				return err
			}
		}
	}
}

func (evaluator *AutomaticEvaluator) scan(ctx context.Context) error {
	definitions, err := evaluator.zones.List(ctx)
	if err != nil {
		return fmt.Errorf("list Zones for Automatic evaluation: %w", err)
	}
	for _, definition := range definitions {
		for _, action := range controlplane.Actions() {
			if err := evaluator.evaluate(ctx, definition.ID, action); err != nil {
				return err
			}
		}
	}
	return nil
}

func (evaluator *AutomaticEvaluator) evaluate(
	ctx context.Context,
	zoneID zone.ID,
	action controlplane.Action,
) error {
	zoneContext, err := zone.NewContext(ctx, zoneID)
	if err != nil {
		return err
	}
	_, _, err = evaluator.actions.InvokeAutomatic(zoneContext, action, time.Now().UTC())
	if errors.Is(err, controlplane.ErrPolicyNotFound) ||
		errors.Is(err, controlplane.ErrNoPendingInput) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("evaluate Automatic %s for Zone %q: %w", action, zoneID, err)
	}
	return nil
}
