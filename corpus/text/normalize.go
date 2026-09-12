package text

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Normalizer interface {
	Profile() string
	Normalize(context.Context, string) (string, error)
}

type StandardNormalizer struct{}

func (StandardNormalizer) Profile() string { return "standard/v1" }

func (StandardNormalizer) Normalize(ctx context.Context, value string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("%w: extracted body is not valid text", ErrInvalid)
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: extracted body is empty", ErrInvalid)
	}
	return value, nil
}
