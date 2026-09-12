package text

import (
	"context"
	"io"

	"github.com/memoria-space/meking/corpus/document"
)

type Extraction struct {
	Title    string
	Body     string
	Format   Format
	Warnings []string
}

type Extractor interface {
	Profile() string
	Extract(context.Context, document.Document, io.Reader) (Extraction, error)
}

type ExtractorResolver interface {
	Resolve(name, mediaType string) (Extractor, int64, error)
}
