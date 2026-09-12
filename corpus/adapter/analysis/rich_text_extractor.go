package analysisadapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	sidecar "github.com/memoria-space/meking/analysis"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
)

type DocumentClient interface {
	ConvertDocument(context.Context, sidecar.DocumentConversionRequest) (sidecar.DocumentConversionResponse, error)
}

type RichTextExtractor struct {
	client       DocumentClient
	maximumBytes int64
}

var _ text.ExtractorResolver = (*RichTextExtractor)(nil)

func NewRichTextExtractor(
	client DocumentClient,
	maximumBytes int64,
) (*RichTextExtractor, error) {
	if client == nil {
		return nil, errors.New("create rich Document Extractor: analysis client is required")
	}

	if maximumBytes <= 0 || maximumBytes > document.MaximumContentBytes {
		return nil, fmt.Errorf(
			"create rich Document Extractor: maximum bytes must be between 1 and %d",
			document.MaximumContentBytes,
		)
	}
	return &RichTextExtractor{
		client:       client,
		maximumBytes: maximumBytes,
	}, nil
}

func (e *RichTextExtractor) Profile() string { return "rich-text/v1" }

func (e *RichTextExtractor) Resolve(name, mediaType string) (text.Extractor, int64, error) {
	if e == nil {
		return nil, 0, text.ErrUnsupported
	}
	if sidecar.SupportsDocumentConversion(name, mediaType) {
		return e, e.maximumBytes, nil
	}
	if sidecar.IsDocumentConversionCandidate(name, mediaType) {
		return nil, 0, fmt.Errorf("%w: rich Document filename and media type do not match", text.ErrInvalid)
	}
	return nil, 0, text.ErrUnsupported
}

func (e *RichTextExtractor) Extract(
	ctx context.Context,
	source document.Document,
	reader io.Reader,
) (text.Extraction, error) {
	if err := ctx.Err(); err != nil {
		return text.Extraction{}, err
	}
	if e == nil || e.client == nil {
		return text.Extraction{}, errors.New("rich Document Extractor is not configured")
	}
	response, err := e.client.ConvertDocument(ctx, sidecar.DocumentConversionRequest{
		Filename: source.Name, Extension: strings.ToLower(filepath.Ext(source.Name)),
		MediaType: source.MediaType, ContentSHA256: source.Digest,
		ContentLength: source.Size, Content: reader,
	})
	if err != nil {
		return text.Extraction{}, err
	}
	if err := response.Validate(); err != nil {
		return text.Extraction{}, fmt.Errorf("validate rich Document analysis response: %w", err)
	}
	return documentExtraction(response, source.Name), nil
}

func documentExtraction(response sidecar.DocumentConversionResponse, sourceName string) text.Extraction {
	title := strings.TrimSuffix(sourceName, filepath.Ext(sourceName))
	if response.Title != nil {
		title = *response.Title
	}
	return text.Extraction{
		Body: response.Markdown, Title: title, Format: text.Markdown,
		Warnings: append([]string(nil), response.Warnings...),
	}
}
