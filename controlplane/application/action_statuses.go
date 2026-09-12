package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/controlplane"
	"github.com/memoria-space/meking/zone"
)

type ActionStatus struct {
	ZoneID       zone.ID
	Action       controlplane.Action
	PendingCount uint64
	PendingSince time.Time
	Policy       controlplane.Policy
}

type ActionStatuses struct {
	policies ActionPolicyReader
	consumer ActionConsumer
}

func NewActionStatuses(
	policies ActionPolicyReader,
	consumer ActionConsumer,
) (*ActionStatuses, error) {
	switch {
	case policies == nil:
		return nil, errors.New("create Action Statuses: Policies are required")
	case consumer == nil:
		return nil, errors.New("create Action Statuses: Consumer is required")
	}
	return &ActionStatuses{
		policies: policies,
		consumer: consumer,
	}, nil
}

func (service *ActionStatuses) Status(
	ctx context.Context,
	action controlplane.Action,
) (ActionStatus, error) {
	if service == nil || service.policies == nil || service.consumer == nil {
		return ActionStatus{}, errors.New("read Action Status: Action Statuses service is required")
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return ActionStatus{}, err
	}
	pending, err := service.consumer.Pending(ctx, action)
	if err != nil {
		return ActionStatus{}, fmt.Errorf("read %s pending input: %w", action, err)
	}
	policy, err := service.policies.Policy(ctx, action)
	if err != nil {
		return ActionStatus{}, fmt.Errorf("read %s Policy: %w", action, err)
	}
	return ActionStatus{
		ZoneID:       zoneID,
		Action:       action,
		PendingCount: pending.Count,
		PendingSince: pending.Since,
		Policy:       policy,
	}, nil
}

func (service *ActionStatuses) List(ctx context.Context) ([]ActionStatus, error) {
	if service == nil {
		return nil, errors.New("list Action Statuses: Action Statuses service is required")
	}
	result := make([]ActionStatus, 0, len(controlplane.Actions()))
	for _, action := range controlplane.Actions() {
		status, err := service.Status(ctx, action)
		if errors.Is(err, controlplane.ErrPolicyNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, status)
	}
	return result, nil
}
