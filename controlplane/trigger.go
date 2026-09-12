package controlplane

import (
	"fmt"
	"time"
)

func ShouldInvoke(
	policy Policy,
	pendingCount uint64,
	pendingSince time.Time,
	now time.Time,
) (bool, error) {
	if policy.Mode != Automatic || pendingCount == 0 {
		return false, nil
	}
	if pendingCount >= policy.MinimumPending {
		return true, nil
	}
	if pendingSince.IsZero() || now.IsZero() || pendingSince.After(now) {
		return false, fmt.Errorf("%w: PendingSince is invalid", ErrInvalidPendingInput)
	}
	return now.Sub(pendingSince) >= policy.MaximumWait, nil
}
