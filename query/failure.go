package query

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// FailureCategory is a stable query outcome that callers can handle without parsing text.
type FailureCategory string

const (
	FailureInvalidInput          FailureCategory = "invalid_input"
	FailureProviderRetryable     FailureCategory = "provider_retryable"
	FailureProviderPermanent     FailureCategory = "provider_permanent"
	FailureInvalidModelResponse  FailureCategory = "invalid_model_response"
	FailureNoPublication         FailureCategory = "no_publication"
	FailurePublicationIncomplete FailureCategory = "publication_incomplete"
	FailureNoEvidence            FailureCategory = "no_evidence"
	FailureBudgetExceeded        FailureCategory = "budget_exceeded"
	FailureInternal              FailureCategory = "internal_failure"
	FailureCancelled             FailureCategory = "cancelled"
)

// Failure exposes safe query diagnostics while retaining its original cause.
type Failure struct {
	// Category identifies the recovery branch without requiring message parsing.
	Category FailureCategory
	// Retryable records whether the external Agent failure is transient.
	Retryable bool
	// StatusCode is the provider HTTP status, or zero when no response was received.
	StatusCode int
	// PartialOutput reports that streaming text reached the caller before the
	// terminal failure. The caller must not transparently restart that stream.
	PartialOutput bool
	diagnostic    string
	cause         error
}

// Error excludes the wrapped cause because a provider body may contain secrets.
func (f *Failure) Error() string {
	if f == nil {
		return "query failed"
	}
	message := "query failed: category=" + string(f.Category)
	if f.StatusCode > 0 {
		message += fmt.Sprintf(" status=%d", f.StatusCode)
	}
	if f.Category == FailureProviderRetryable || f.Category == FailureProviderPermanent {
		message += fmt.Sprintf(" retryable=%t", f.Retryable)
	}
	if f.PartialOutput {
		message += " partial_output=true"
	}
	if f.diagnostic != "" {
		message += " diagnostic=" + strconv.Quote(f.diagnostic)
	}
	return message
}

// MarkPartialOutput copies a normalized failure and records that answer text
// was already delivered. Non-query errors are converted to an internal failure.
func MarkPartialOutput(err error) *Failure {
	var failure *Failure
	if !errors.As(err, &failure) {
		failure = NewInternalFailure(err)
	}
	copy := *failure
	copy.PartialOutput = true
	return &copy
}

// Unwrap preserves programmatic access to the original cause without logging it.
func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.cause
}

// Diagnostic returns the safe recovery guidance attached to the failure.
func (f *Failure) Diagnostic() string {
	if f == nil {
		return ""
	}
	return f.diagnostic
}

// NewInvalidInputFailure reports a request or configuration that cannot be executed.
// The diagnostic must be fixed, safe guidance rather than raw input or provider text.
func NewInvalidInputFailure(diagnostic string, cause error) *Failure {
	return newFailure(FailureInvalidInput, diagnostic, false, 0, cause)
}

// NewProviderFailure maps provider policy metadata into one query failure category.
func NewProviderFailure(retryable bool, statusCode int, cause error) *Failure {
	category := FailureProviderPermanent
	diagnostic := "check the model configuration and provider status"
	if retryable {
		category = FailureProviderRetryable
		diagnostic = "retry the query after the provider recovers"
	}
	return newFailure(category, diagnostic, retryable, statusCode, cause)
}

// NewInvalidModelResponseFailure reports a response that violates a required schema.
func NewInvalidModelResponseFailure(cause error) *Failure {
	return newFailure(
		FailureInvalidModelResponse,
		"retry the query or inspect the selected model output",
		false,
		0,
		cause,
	)
}

// NewNoPublicationFailure reports that no current immutable publication is
// available for the requested query kind.
func NewNoPublicationFailure(cause error) *Failure {
	return newFailure(
		FailureNoPublication,
		"publish indexed data before querying",
		false,
		0,
		cause,
	)
}

// NewPublicationIncompleteFailure reports that a selected publication cannot
// be opened with all mandatory derived data required by its query kind.
func NewPublicationIncompleteFailure(cause error) *Failure {
	return newFailure(
		FailurePublicationIncomplete,
		"rebuild and publish a complete index before retrying",
		false,
		0,
		cause,
	)
}

// NewNoEvidenceFailure reports a successful retrieval that selected no facts
// from the fixed publication.
func NewNoEvidenceFailure(cause error) *Failure {
	return newFailure(
		FailureNoEvidence,
		"ask a question covered by the indexed sources",
		false,
		0,
		cause,
	)
}

// NewBudgetExceededFailure reports that a configured hard query resource limit
// prevents the current stage from completing.
func NewBudgetExceededFailure(cause error) *Failure {
	return newFailure(
		FailureBudgetExceeded,
		"increase the applicable query budget or narrow the request",
		false,
		0,
		cause,
	)
}

// NewInternalFailure reports a safe catch-all for local runtime and adapter failures.
func NewInternalFailure(cause error) *Failure {
	return newFailure(
		FailureInternal,
		"inspect local query diagnostics and retry",
		false,
		0,
		cause,
	)
}

// NewCancelledFailure reports cancellation or deadline expiry requested by the caller.
func NewCancelledFailure(cause error) *Failure {
	return newFailure(FailureCancelled, "query was cancelled", false, 0, cause)
}

func newFailure(
	category FailureCategory,
	diagnostic string,
	retryable bool,
	statusCode int,
	cause error,
) *Failure {
	return &Failure{
		Category: category, Retryable: retryable, StatusCode: statusCode,
		diagnostic: strings.TrimSpace(diagnostic), cause: cause,
	}
}
