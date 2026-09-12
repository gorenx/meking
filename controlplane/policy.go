package controlplane

import (
	"fmt"
	"time"
)

type PolicyMode string

const (
	Automatic PolicyMode = "automatic"
	Manual    PolicyMode = "manual"
	Suspended PolicyMode = "suspended"
)

type Policy struct {
	Action         Action
	Mode           PolicyMode
	MinimumPending uint64
	MaximumWait    time.Duration
	Revision       uint64
	UpdatedAt      time.Time
}

func NewPolicy(
	action Action,
	mode PolicyMode,
	minimumPending uint64,
	maximumWait time.Duration,
	updatedAt time.Time,
) (Policy, error) {
	return RestorePolicy(Policy{
		Action:         action,
		Mode:           mode,
		MinimumPending: minimumPending,
		MaximumWait:    maximumWait,
		Revision:       1,
		UpdatedAt:      updatedAt,
	})
}

func RestorePolicy(value Policy) (Policy, error) {
	if _, err := ParseAction(string(value.Action)); err != nil {
		return Policy{}, fmt.Errorf("%w: %v", ErrInvalidPolicy, err)
	}
	switch value.Mode {
	case Automatic:
		if value.MinimumPending == 0 || value.MaximumWait <= 0 {
			return Policy{}, fmt.Errorf(
				"%w: Automatic requires positive MinimumPending and MaximumWait",
				ErrInvalidPolicy,
			)
		}
	case Manual, Suspended:
		if value.MinimumPending != 0 || value.MaximumWait != 0 {
			return Policy{}, fmt.Errorf(
				"%w: Manual and Suspended cannot define Automatic thresholds",
				ErrInvalidPolicy,
			)
		}
	default:
		return Policy{}, fmt.Errorf("%w: unsupported mode %q", ErrInvalidPolicy, value.Mode)
	}
	if value.Revision == 0 || value.UpdatedAt.IsZero() {
		return Policy{}, ErrInvalidPolicy
	}
	value.UpdatedAt = value.UpdatedAt.UTC()
	return value, nil
}

func (policy Policy) Change(
	mode PolicyMode,
	minimumPending uint64,
	maximumWait time.Duration,
	updatedAt time.Time,
) (Policy, bool, error) {
	candidate := Policy{
		Action:         policy.Action,
		Mode:           mode,
		MinimumPending: minimumPending,
		MaximumWait:    maximumWait,
		Revision:       policy.Revision,
		UpdatedAt:      updatedAt,
	}
	validated, err := RestorePolicy(candidate)
	if err != nil {
		return Policy{}, false, err
	}
	if policy.Mode == validated.Mode &&
		policy.MinimumPending == validated.MinimumPending &&
		policy.MaximumWait == validated.MaximumWait {
		return policy, false, nil
	}
	validated.Revision++
	return validated, true, nil
}
