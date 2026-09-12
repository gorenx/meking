package analysis

import (
	"errors"
	"fmt"
	"strings"
)

// FailureKind distinguishes an unavailable analysis runtime from an operation
// that reached the runtime but did not complete.
type FailureKind string

const (
	// FailureUnavailable means the local analysis process or contract cannot be used.
	FailureUnavailable FailureKind = "analysis_unavailable"
	// FailureFailed means an authenticated analysis operation failed after startup.
	FailureFailed FailureKind = "analysis_failed"
)

// Failure is a stable, secret-safe cross-language failure. The wrapped cause
// remains available to in-process diagnostics but is never copied into Error.
type Failure struct {
	Kind      FailureKind
	Operation string
	Retryable bool
	cause     error
}

// Error returns only stable fields and never includes child stderr, response
// bodies, command arguments, environment variables, or Python tracebacks.
func (f *Failure) Error() string {
	if f == nil {
		return ""
	}
	operation := strings.TrimSpace(f.Operation)
	if operation == "" {
		operation = "analysis"
	}
	return fmt.Sprintf("%s: operation=%s retryable=%t", f.Kind, operation, f.Retryable)
}

// Unwrap preserves programmatic inspection without making the cause log-safe.
func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.cause
}

func unavailable(operation string, retryable bool, cause error) error {
	if cause == nil {
		cause = errors.New("analysis runtime is unavailable")
	}
	return &Failure{
		Kind: FailureUnavailable, Operation: operation,
		Retryable: retryable, cause: cause,
	}
}

func failed(operation string, retryable bool, cause error) error {
	if cause == nil {
		cause = errors.New("analysis operation failed")
	}
	return &Failure{
		Kind: FailureFailed, Operation: operation,
		Retryable: retryable, cause: cause,
	}
}
