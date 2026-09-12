package drift

import (
	"context"
	"errors"

	querybase "github.com/memoria-space/meking/query"
)

func normalizePublicationFailure(err error) *querybase.Failure {
	if isCancellation(err) {
		return querybase.NewCancelledFailure(err)
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewPublicationIncompleteFailure(err)
}

func normalizeFailure(err error) *querybase.Failure {
	if isCancellation(err) {
		return querybase.NewCancelledFailure(err)
	}
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewInternalFailure(err)
}

func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
