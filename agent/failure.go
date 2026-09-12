package agent

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/openai/openai-go/v3"
)

type ProviderError struct {
	StatusCode int
	Retryable  bool
	cause      error
}

func (err *ProviderError) Error() string {
	if err.StatusCode > 0 {
		return fmt.Sprintf(
			"provider request failed: status=%d retryable=%t",
			err.StatusCode,
			err.Retryable,
		)
	}
	return fmt.Sprintf("provider request failed: retryable=%t", err.Retryable)
}

func (err *ProviderError) ProviderStatusCode() int {
	return err.StatusCode
}

func (err *ProviderError) ProviderRetryable() bool {
	return err.Retryable
}

func (err *ProviderError) Unwrap() error {
	return err.cause
}

func newProviderError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return &ProviderError{
			StatusCode: apiErr.StatusCode,
			Retryable:  providerRetryable(apiErr.StatusCode),
			cause:      err,
		}
	}
	var networkError net.Error
	return &ProviderError{
		Retryable: errors.As(err, &networkError),
		cause:     err,
	}
}

func providerRetryable(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusConflict ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}
