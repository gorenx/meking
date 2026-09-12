// Package modeladapter contains Query-internal mapping shared by model-facing
// adapters. It owns no provider client and exposes no public application API.
package modeladapter

import (
	"context"
	"errors"

	querybase "github.com/memoria-space/meking/query"
)

type providerFailure interface {
	ProviderStatusCode() int
	ProviderRetryable() bool
}

// Failure maps provider-neutral model errors into Query's stable failure
// categories while preserving cancellation and an existing Query failure.
func Failure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	var provider providerFailure
	if errors.As(err, &provider) {
		return querybase.NewProviderFailure(
			provider.ProviderRetryable(),
			provider.ProviderStatusCode(),
			err,
		)
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewInternalFailure(err)
}
