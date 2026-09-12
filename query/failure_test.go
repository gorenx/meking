package query

import (
	"errors"
	"strings"
	"testing"
)

func TestFailureKeepsTypedMetadataWithoutLeakingCause(t *testing.T) {
	cause := errors.New("provider body contains secret-token")
	failure := NewProviderFailure(true, 429, cause)
	if failure.Category != FailureProviderRetryable || !failure.Retryable || failure.StatusCode != 429 {
		t.Fatalf("failure = %#v", failure)
	}
	if !errors.Is(failure, cause) {
		t.Fatal("failure does not unwrap to its cause")
	}
	if strings.Contains(failure.Error(), "secret-token") ||
		!strings.Contains(failure.Error(), "category=provider_retryable") ||
		!strings.Contains(failure.Error(), "status=429") {
		t.Fatalf("Error() = %q", failure.Error())
	}
}

func TestMarkPartialOutputCopiesNormalizedFailure(t *testing.T) {
	original := NewProviderFailure(true, 429, errors.New("secret response"))
	partial := MarkPartialOutput(original)
	if partial == original || original.PartialOutput || !partial.PartialOutput ||
		partial.Category != FailureProviderRetryable ||
		!strings.Contains(partial.Error(), "partial_output=true") ||
		strings.Contains(partial.Error(), "secret response") {
		t.Fatalf("original/partial = %#v/%#v", original, partial)
	}
	local := MarkPartialOutput(errors.New("writer failed"))
	if local.Category != FailureInternal || !local.PartialOutput {
		t.Fatalf("local partial = %#v", local)
	}
}

func TestFailureConstructorsExposeStableCategories(t *testing.T) {
	tests := []struct {
		name     string
		failure  *Failure
		category FailureCategory
	}{
		{name: "invalid input", failure: NewInvalidInputFailure("invalid", nil), category: FailureInvalidInput},
		{name: "provider permanent", failure: NewProviderFailure(false, 401, nil), category: FailureProviderPermanent},
		{name: "invalid model response", failure: NewInvalidModelResponseFailure(nil), category: FailureInvalidModelResponse},
		{name: "no publication", failure: NewNoPublicationFailure(nil), category: FailureNoPublication},
		{name: "publication incomplete", failure: NewPublicationIncompleteFailure(nil), category: FailurePublicationIncomplete},
		{name: "no evidence", failure: NewNoEvidenceFailure(nil), category: FailureNoEvidence},
		{name: "budget exceeded", failure: NewBudgetExceededFailure(nil), category: FailureBudgetExceeded},
		{name: "internal", failure: NewInternalFailure(nil), category: FailureInternal},
		{name: "cancelled", failure: NewCancelledFailure(nil), category: FailureCancelled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.failure.Category != test.category || test.failure.Diagnostic() == "" {
				t.Fatalf("failure = %#v, diagnostic = %q", test.failure, test.failure.Diagnostic())
			}
		})
	}
}
