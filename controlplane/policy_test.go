package controlplane

import (
	"errors"
	"testing"
	"time"
)

func TestAutomaticPolicyRequiresMinimumPending(t *testing.T) {
	now := time.Now()
	if _, err := NewPolicy(
		ConvertDocument,
		Automatic,
		0,
		time.Minute,
		now,
	); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("NewPolicy() error = %v", err)
	}
}

func TestAutomaticPolicyRequiresMaximumWait(t *testing.T) {
	now := time.Now()
	if _, err := NewPolicy(
		ConvertDocument,
		Automatic,
		1,
		0,
		now,
	); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("NewPolicy() error = %v", err)
	}
}

func TestShouldInvokeAutomaticByQuantity(t *testing.T) {
	now := time.Now().UTC()
	policy, err := NewPolicy(CreateTextUnits, Automatic, 3, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	invoke, err := ShouldInvoke(policy, 3, now, now)
	if err != nil || !invoke {
		t.Fatalf("ShouldInvoke(quantity) = %v, %v", invoke, err)
	}
}

func TestShouldInvokeAutomaticByWait(t *testing.T) {
	now := time.Now().UTC()
	policy, err := NewPolicy(CreateTextUnits, Automatic, 3, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	invoke, err := ShouldInvoke(policy, 1, now.Add(-time.Minute), now)
	if err != nil || !invoke {
		t.Fatalf("ShouldInvoke(wait) = %v, %v", invoke, err)
	}
}

func TestManualPolicyDoesNotInvokeAutomatically(t *testing.T) {
	now := time.Now().UTC()
	policy, err := NewPolicy(ExtractKnowledge, Manual, 0, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	invoke, err := ShouldInvoke(policy, 9, now, now)
	if err != nil || invoke {
		t.Fatalf("ShouldInvoke(manual) = %v, %v", invoke, err)
	}
}

func TestSuspendedPolicyDoesNotInvokeAutomatically(t *testing.T) {
	now := time.Now().UTC()
	policy, err := NewPolicy(ExtractKnowledge, Suspended, 0, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	invoke, err := ShouldInvoke(policy, 9, now, now)
	if err != nil || invoke {
		t.Fatalf("ShouldInvoke(suspended) = %v, %v", invoke, err)
	}
}
