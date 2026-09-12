package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/controlplane"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
)

type Consumer struct {
	Pending func(context.Context) (controlapplication.PendingInput, error)
	Start   func(context.Context) error
	Retry   func(context.Context) error
}

type Router struct {
	consumers map[controlplane.Action]Consumer
}

var _ controlapplication.ActionConsumer = (*Router)(nil)

func NewRouter(consumers map[controlplane.Action]Consumer) (*Router, error) {
	if len(consumers) != len(controlplane.Actions()) {
		return nil, errors.New("create Action Consumer router: every Action is required")
	}
	copied := make(map[controlplane.Action]Consumer, len(consumers))
	for _, action := range controlplane.Actions() {
		consumer := consumers[action]
		if consumer.Pending == nil || consumer.Start == nil || consumer.Retry == nil {
			return nil, fmt.Errorf(
				"create Action Consumer router: %s Consumer is required",
				action,
			)
		}
		copied[action] = consumer
	}
	return &Router{
		consumers: copied,
	}, nil
}

func (router *Router) Pending(
	ctx context.Context,
	action controlplane.Action,
) (controlapplication.PendingInput, error) {
	if router == nil || router.consumers == nil {
		return controlapplication.PendingInput{}, errors.New(
			"read pending Action input: Consumer router is required",
		)
	}
	consumer, found := router.consumers[action]
	if !found {
		return controlapplication.PendingInput{}, fmt.Errorf(
			"read pending %s input: domain Consumer is required",
			action,
		)
	}
	pending, err := consumer.Pending(ctx)
	if err != nil {
		return controlapplication.PendingInput{}, fmt.Errorf(
			"read pending %s input: %w",
			action,
			err,
		)
	}
	return controlapplication.PendingInput{
		Count: pending.Count,
		Since: pending.Since,
	}, nil
}

func (router *Router) Start(
	ctx context.Context,
	action controlplane.Action,
) error {
	if router == nil || router.consumers == nil {
		return errors.New("start Action: Consumer router is required")
	}
	consumer, found := router.consumers[action]
	if !found {
		return fmt.Errorf("start %s: domain Consumer is required", action)
	}
	if err := consumer.Start(ctx); err != nil {
		return fmt.Errorf("start %s: %w", action, err)
	}
	return nil
}

func (router *Router) Retry(
	ctx context.Context,
	action controlplane.Action,
) error {
	if router == nil || router.consumers == nil {
		return errors.New("retry Action: Consumer router is required")
	}
	consumer, found := router.consumers[action]
	if !found {
		return fmt.Errorf("retry %s: domain Consumer is required", action)
	}
	if err := consumer.Retry(ctx); err != nil {
		return fmt.Errorf("retry %s: %w", action, err)
	}
	return nil
}
