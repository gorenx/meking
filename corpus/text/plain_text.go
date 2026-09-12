package text

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/memoria-space/meking/corpus/document"
)

type PlainTextExtractor struct{}

func (PlainTextExtractor) Profile() string { return "plain-text/utf8/v1" }

func (PlainTextExtractor) Extract(
	ctx context.Context,
	value document.Document,
	reader io.Reader,
) (Extraction, error) {
	if err := ctx.Err(); err != nil {
		return Extraction{}, err
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return Extraction{}, err
	}
	if !utf8.Valid(data) {
		return Extraction{}, fmt.Errorf("%w: Document %q is not valid UTF-8", ErrInvalid, value.ID)
	}
	title := strings.TrimSuffix(value.Name, filepath.Ext(value.Name))
	return Extraction{Title: title, Body: string(data), Format: PlainText}, nil
}

type ExtractorResolverFunc func(name, mediaType string) (Extractor, int64, error)

func (resolve ExtractorResolverFunc) Resolve(name, mediaType string) (Extractor, int64, error) {
	if resolve == nil {
		return nil, 0, ErrUnsupported
	}
	return resolve(name, mediaType)
}
