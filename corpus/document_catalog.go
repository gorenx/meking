package corpus

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/corpus/document"
)

const MaximumDocumentCatalogPageSize = 200
const DefaultDocumentCatalogPageSize = 50

// DocumentCatalogEntry is one Project Document together with whether the
// current Zone has already selected it. Content locations remain internal.
type DocumentCatalogEntry struct {
	Document document.Document
	Selected bool
}

type DocumentCatalogPage struct {
	Documents []DocumentCatalogEntry
	Next      *document.Page
}

type DocumentCatalogReader interface {
	BrowseDocuments(context.Context, document.Page) (DocumentCatalogPage, error)
}

func (s *Service) BrowseDocuments(
	ctx context.Context,
	page document.Page,
) (DocumentCatalogPage, error) {
	if s == nil || s.documentCatalog == nil {
		return DocumentCatalogPage{}, errors.New("Corpus Document Catalog is not configured")
	}
	if page.Offset < 0 || page.Limit <= 0 || page.Limit > MaximumDocumentCatalogPageSize {
		return DocumentCatalogPage{}, document.ErrInvalid
	}
	return s.documentCatalog.BrowseDocuments(ctx, page)
}
