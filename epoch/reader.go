package epoch

import (
	"context"
	"errors"
	"fmt"
)

// Reader exposes Current and exact historical Epochs without requiring the
// publication coordinator's Knowledge and readiness dependencies. Delivery
// compositions keep it open only for the lifetime of their persistence pool.
type Reader struct {
	// store is the fact source for the single Current selection and immutable
	// Epoch rows. Reader never writes through it.
	store ReadStore
}

// NewReader creates the read-only Epoch application used by Query and other
// consumers that already have a published identity.
func NewReader(store ReadStore) (*Reader, error) {
	if store == nil {
		return nil, errors.New("create Epoch Reader: store is required")
	}
	return &Reader{store: store}, nil
}

// Current returns the Epoch selected at the start of this read. Callers retain
// only this immutable value and use exact IDs for subsequent lazy reads.
func (r *Reader) Current(ctx context.Context) (Epoch, error) {
	if r == nil || r.store == nil {
		return Epoch{}, errors.New("Epoch Reader is required")
	}
	value, err := r.store.Current(ctx)
	if err != nil {
		return Epoch{}, err
	}
	if err := ValidateEpoch(value); err != nil {
		return Epoch{}, fmt.Errorf("%w: Current value is invalid: %v", ErrEpochDataIntegrity, err)
	}
	return value, nil
}

// Epoch returns one exact historical publication without consulting Current.
func (r *Reader) Epoch(ctx context.Context, id ID) (Epoch, error) {
	if r == nil || r.store == nil {
		return Epoch{}, errors.New("Epoch Reader is required")
	}
	if id <= 0 {
		return Epoch{}, fmt.Errorf("%w: requested ID must be positive", ErrInvalidEpoch)
	}
	value, err := r.store.Load(ctx, id)
	if err != nil {
		return Epoch{}, err
	}
	if err := ValidateEpoch(value); err != nil {
		return Epoch{}, fmt.Errorf("%w: Epoch %d is invalid: %v", ErrEpochDataIntegrity, id, err)
	}
	return value, nil
}
