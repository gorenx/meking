package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/controlplane"
	"github.com/memoria-space/meking/transaction"
)

type PolicyStore interface {
	LoadPolicy(ctx context.Context, action controlplane.Action) (controlplane.Policy, error)
	SavePolicy(ctx context.Context, expectedRevision uint64, policy controlplane.Policy) error
	ListPolicies(ctx context.Context) ([]controlplane.Policy, error)
}

type Policies struct {
	transactions transaction.Tx
	store        PolicyStore
}

type PublishPolicyInput struct {
	Action           controlplane.Action
	Mode             controlplane.PolicyMode
	MinimumPending   uint64
	MaximumWait      time.Duration
	ExpectedRevision uint64
	PublishedAt      time.Time
}

func NewPolicies(
	transactions transaction.Tx,
	store PolicyStore,
) (*Policies, error) {
	if transactions == nil {
		return nil, errors.New("create Policies service: Transactions are required")
	}
	if store == nil {
		return nil, errors.New("create Policies service: Store is required")
	}
	return &Policies{
		transactions: transactions,
		store:        store,
	}, nil
}

func (service *Policies) Publish(
	ctx context.Context,
	input PublishPolicyInput,
) (controlplane.Policy, error) {
	if service == nil || service.transactions == nil || service.store == nil {
		return controlplane.Policy{}, errors.New("publish Control Policy: Policies service is required")
	}
	if _, err := controlplane.ParseAction(string(input.Action)); err != nil {
		return controlplane.Policy{}, err
	}
	var result controlplane.Policy
	err := service.transactions.WithTx(ctx, func(ctx context.Context) error {
		current, err := service.store.LoadPolicy(ctx, input.Action)
		switch {
		case errors.Is(err, controlplane.ErrPolicyNotFound):
			if input.ExpectedRevision != 0 {
				return controlplane.ErrPolicyConflict
			}
			created, err := controlplane.NewPolicy(
				input.Action,
				input.Mode,
				input.MinimumPending,
				input.MaximumWait,
				input.PublishedAt,
			)
			if err != nil {
				return err
			}
			if err := service.store.SavePolicy(ctx, 0, created); err != nil {
				return err
			}
			result = created
			return nil
		case err != nil:
			return err
		}
		if current.Revision != input.ExpectedRevision {
			return controlplane.ErrPolicyConflict
		}
		updated, changed, err := current.Change(
			input.Mode,
			input.MinimumPending,
			input.MaximumWait,
			input.PublishedAt,
		)
		if err != nil {
			return err
		}
		if changed {
			if err := service.store.SavePolicy(ctx, current.Revision, updated); err != nil {
				return err
			}
		}
		result = updated
		return nil
	})
	if err != nil {
		return controlplane.Policy{}, fmt.Errorf("publish %s Policy: %w", input.Action, err)
	}
	return result, nil
}

func (service *Policies) Policy(
	ctx context.Context,
	action controlplane.Action,
) (controlplane.Policy, error) {
	if service == nil || service.store == nil {
		return controlplane.Policy{}, errors.New("read Control Policy: Policies service is required")
	}
	if _, err := controlplane.ParseAction(string(action)); err != nil {
		return controlplane.Policy{}, err
	}
	return service.store.LoadPolicy(ctx, action)
}

func (service *Policies) List(ctx context.Context) ([]controlplane.Policy, error) {
	if service == nil || service.store == nil {
		return nil, errors.New("list Control Policies: Policies service is required")
	}
	return service.store.ListPolicies(ctx)
}
