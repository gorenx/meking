package analysisadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/memoria-space/meking/analysis"
	"github.com/memoria-space/meking/corpus/text"
)

type documentClientStub struct{}

func (documentClientStub) ConvertDocument(
	context.Context,
	analysis.DocumentConversionRequest,
) (analysis.DocumentConversionResponse, error) {
	return analysis.DocumentConversionResponse{}, nil
}

func TestRichTextExtractorResolvesOnlyMatchingRichDocumentFormats(t *testing.T) {
	t.Parallel()

	extractor, err := NewRichTextExtractor(documentClientStub{}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if resolved, maximumBytes, err := extractor.Resolve("source.html", "text/html"); err != nil || resolved != extractor || maximumBytes != 4096 {
		t.Fatalf("Resolve(valid) = %T, %d, %v", resolved, maximumBytes, err)
	}
	if _, _, err := extractor.Resolve("source.txt", "text/html"); !errors.Is(err, text.ErrInvalid) {
		t.Fatalf("Resolve(mismatch) error = %v, want ErrInvalid", err)
	}
	if _, _, err := extractor.Resolve("source.txt", "text/plain"); !errors.Is(err, text.ErrUnsupported) {
		t.Fatalf("Resolve(plain) error = %v, want ErrUnsupported", err)
	}
}

func TestDocumentExtractionUsesSourceNameWhenConverterHasNoTitle(t *testing.T) {
	t.Parallel()

	extracted := documentExtraction(analysis.DocumentConversionResponse{Markdown: "# 内容"}, "季度报告.pdf")
	if extracted.Title != "季度报告" || extracted.Body != "# 内容" || extracted.Format != text.Markdown {
		t.Fatalf("documentExtraction() = %#v", extracted)
	}

	provided := "转换标题"
	extracted = documentExtraction(
		analysis.DocumentConversionResponse{Markdown: "# 内容", Title: &provided},
		"季度报告.pdf",
	)
	if extracted.Title != provided {
		t.Fatalf("documentExtraction() Title = %q, want %q", extracted.Title, provided)
	}
}
