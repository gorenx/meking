package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/controlplane"
)

type actionPolicyStub struct {
	policy controlplane.Policy
}

func (stub actionPolicyStub) Policy(context.Context, controlplane.Action) (controlplane.Policy, error) {
	return stub.policy, nil
}

type actionConsumerStub struct {
	pending PendingInput
	action  controlplane.Action
	starts  int
	retries int
}

func (stub *actionConsumerStub) Pending(_ context.Context, action controlplane.Action) (PendingInput, error) {
	stub.action = action
	return stub.pending, nil
}

func (stub *actionConsumerStub) Start(_ context.Context, action controlplane.Action) error {
	stub.action = action
	stub.starts++
	return nil
}

func (stub *actionConsumerStub) Retry(_ context.Context, action controlplane.Action) error {
	stub.action = action
	stub.retries++
	return nil
}

func TestActionsInvokeManualRetriesOneAction(t *testing.T) {
	action := controlplane.ExtractKnowledge
	consumer := &actionConsumerStub{pending: PendingInput{Count: 1, Since: time.Now().UTC()}}
	service := newActions(t, action, controlplane.Manual, consumer)
	invocation, err := service.InvokeManual(t.Context(), action)
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Action != action || consumer.action != action || consumer.retries != 1 || consumer.starts != 0 {
		t.Fatalf("Invocation = %#v, Consumer = %#v", invocation, consumer)
	}
}

func TestActionsInvokeManualRequiresManualPolicy(t *testing.T) {
	action := controlplane.ExtractKnowledge
	consumer := &actionConsumerStub{pending: PendingInput{Count: 1, Since: time.Now().UTC()}}
	service := newActions(t, action, controlplane.Automatic, consumer)
	_, err := service.InvokeManual(t.Context(), action)
	if !errors.Is(err, controlplane.ErrActionNotInvocable) {
		t.Fatalf("InvokeManual() error = %v", err)
	}
	if consumer.retries != 0 {
		t.Fatalf("Retry calls = %d", consumer.retries)
	}
}

func TestActionsInvokeAutomaticStartsWhenThresholdIsMet(t *testing.T) {
	action := controlplane.CreateTextUnits
	consumer := &actionConsumerStub{pending: PendingInput{Count: 3, Since: time.Now().UTC()}}
	service := newActions(t, action, controlplane.Automatic, consumer)
	invocation, invoked, err := service.InvokeAutomatic(t.Context(), action, time.Now().UTC())
	if err != nil || !invoked || invocation.Action != action {
		t.Fatalf("InvokeAutomatic() = %#v, %t, %v", invocation, invoked, err)
	}
	if consumer.starts != 1 || consumer.retries != 0 {
		t.Fatalf("Consumer = %#v", consumer)
	}
}

func TestActionsDoNotInvokeWithoutPendingInput(t *testing.T) {
	action := controlplane.ExtractKnowledge
	consumer := &actionConsumerStub{}
	service := newActions(t, action, controlplane.Manual, consumer)
	_, err := service.InvokeManual(t.Context(), action)
	if !errors.Is(err, controlplane.ErrNoPendingInput) {
		t.Fatalf("InvokeManual() error = %v", err)
	}
	if consumer.retries != 0 {
		t.Fatalf("Retry calls = %d", consumer.retries)
	}
}

func newActions(t *testing.T, action controlplane.Action, mode controlplane.PolicyMode, consumer *actionConsumerStub) *Actions {
	t.Helper()
	minimumPending := uint64(0)
	maximumWait := time.Duration(0)
	if mode == controlplane.Automatic {
		minimumPending = 3
		maximumWait = time.Minute
	}
	policy, err := controlplane.NewPolicy(action, mode, minimumPending, maximumWait, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewActions(ActionDependencies{
		Policies: actionPolicyStub{policy: policy},
		Consumer: consumer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
