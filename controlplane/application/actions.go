package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/controlplane"
)

type ActionPolicyReader interface {
	Policy(context.Context, controlplane.Action) (controlplane.Policy, error)
}

type ActionDependencies struct {
	Policies ActionPolicyReader
	Consumer ActionConsumer
}

type Actions struct {
	policies ActionPolicyReader
	consumer ActionConsumer
}

type Invocation struct {
	Action controlplane.Action
}

func NewActions(dependencies ActionDependencies) (*Actions, error) {
	switch {
	case dependencies.Policies == nil:
		return nil, errors.New("create Actions service: Policies are required")
	case dependencies.Consumer == nil:
		return nil, errors.New("create Actions service: Consumer is required")
	}
	return &Actions{
		policies: dependencies.Policies,
		consumer: dependencies.Consumer,
	}, nil
}

func (service *Actions) InvokeManual(
	ctx context.Context,
	action controlplane.Action,
) (Invocation, error) {
	if service == nil {
		return Invocation{}, errors.New("invoke Action manually: Actions service is required")
	}
	policy, err := service.policies.Policy(ctx, action)
	if err != nil {
		return Invocation{}, fmt.Errorf("read %s Policy: %w", action, err)
	}
	if policy.Mode != controlplane.Manual {
		return Invocation{}, fmt.Errorf(
			"%w: %s Policy is %s",
			controlplane.ErrActionNotInvocable,
			action,
			policy.Mode,
		)
	}
	pending, err := service.consumer.Pending(ctx, action)
	if err != nil {
		return Invocation{}, err
	}
	if pending.Count == 0 {
		return Invocation{}, controlplane.ErrNoPendingInput
	}
	if err := service.consumer.Retry(ctx, action); err != nil {
		return Invocation{}, fmt.Errorf("invoke %s: %w", action, err)
	}
	return Invocation{Action: action}, nil
}

func (service *Actions) InvokeAutomatic(
	ctx context.Context,
	action controlplane.Action,
	now time.Time,
) (Invocation, bool, error) {
	if service == nil {
		return Invocation{}, false, errors.New("invoke Action automatically: Actions service is required")
	}
	policy, err := service.policies.Policy(ctx, action)
	if err != nil {
		return Invocation{}, false, fmt.Errorf("read %s Policy: %w", action, err)
	}
	pending, err := service.consumer.Pending(ctx, action)
	if err != nil {
		return Invocation{}, false, err
	}
	invoke, err := controlplane.ShouldInvoke(policy, pending.Count, pending.Since, now)
	if err != nil || !invoke {
		return Invocation{}, false, err
	}
	if err := service.consumer.Start(ctx, action); err != nil {
		return Invocation{}, true, fmt.Errorf("invoke %s: %w", action, err)
	}
	return Invocation{Action: action}, true, nil
}
