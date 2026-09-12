package sqlite

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/zone"
)

type DocumentCatalog struct {
	corpus *Store
}

var _ corpus.DocumentCatalogReader = (*DocumentCatalog)(nil)

func NewDocumentCatalog(store *Store) (*DocumentCatalog, error) {
	if store == nil || store.database == nil {
		return nil, errors.New("create Document Catalog: Corpus Store is required")
	}
	return &DocumentCatalog{corpus: store}, nil
}

func (catalog *DocumentCatalog) BrowseDocuments(
	ctx context.Context,
	page document.Page,
) (corpus.DocumentCatalogPage, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return corpus.DocumentCatalogPage{}, err
	}
	rows, err := newStatements(catalog.corpus.database).BrowseDocuments(
		ctx,
		db.BrowseDocumentsParams{
			ZoneID: string(zoneID), Limit: int64(page.Limit + 1), Offset: int64(page.Offset),
		},
	)
	if err != nil {
		return corpus.DocumentCatalogPage{}, classifySQLite("browse Document Catalog", err)
	}
	count := min(len(rows), page.Limit)
	documents := make([]corpus.DocumentCatalogEntry, count)
	for index := 0; index < count; index++ {
		row := rows[index]
		value, err := restoreDocument(documentRecord{
			ID: row.ID, Name: row.Name, MediaType: row.MediaType,
			Size: row.Size, Digest: row.Digest,
		})
		if err != nil {
			return corpus.DocumentCatalogPage{}, err
		}
		documents[index] = corpus.DocumentCatalogEntry{
			Document: value, Selected: row.Selected != 0,
		}
	}
	result := corpus.DocumentCatalogPage{Documents: documents}
	if len(rows) > page.Limit {
		result.Next = &document.Page{Offset: page.Offset + page.Limit, Limit: page.Limit}
	}
	return result, nil
}
