package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	db2 "github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/zone"
)

type DocumentRepository struct {
	corpus *Store
}

var _ document.Repository = (*DocumentRepository)(nil)

func NewDocumentRepository(store *Store) (*DocumentRepository, error) {
	if store == nil || store.database == nil {
		return nil, errors.New("create Document Repository: Corpus Store is required")
	}
	return &DocumentRepository{corpus: store}, nil
}

func (r *DocumentRepository) Save(
	ctx context.Context,
	value document.Document,
	location document.Location,
) (document.SaveResult, error) {
	if location == "" {
		return document.SaveResult{}, fmt.Errorf("%w: Document Location is required", document.ErrInvalid)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return document.SaveResult{}, err
	}
	zoneIDValue := string(zoneID)
	var result document.SaveResult
	err = r.corpus.write(ctx, zoneIDValue, func(statements statements) error {
		inserted, err := statements.SaveDocument(ctx, db2.SaveDocumentParams{
			ID: string(value.ID), Name: value.Name, MediaType: value.MediaType,
			Size: value.Size, Digest: value.Digest, Location: string(location),
		})
		if err != nil {
			return classifySQLite("save Document", err)
		}
		row, err := statements.GetDocumentByDigest(ctx, value.Digest)
		if errors.Is(err, sql.ErrNoRows) {
			return document.ErrContentConflict
		}
		if err != nil {
			return classifySQLite("read Document by Digest", err)
		}
		stored, err := restoreLocatedDocument(documentRecord{
			ID: row.ID, Name: row.Name, MediaType: row.MediaType,
			Size: row.Size, Digest: row.Digest, Location: row.Location,
		})
		if err != nil {
			return err
		}
		if stored.Document.Size != value.Size {
			return fmt.Errorf("%w: Digest identifies different content sizes", document.ErrStorageIntegrity)
		}
		result = document.SaveResult{
			Document: stored.Document, Location: stored.Location,
			DocumentCreated: inserted == 1,
		}
		return nil
	})
	if err != nil {
		return document.SaveResult{}, err
	}
	return result, nil
}

func (r *DocumentRepository) Associate(
	ctx context.Context,
	ids []document.ID,
) ([]document.LocatedDocument, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, err
	}
	zoneIDValue := string(zoneID)
	result := make([]document.LocatedDocument, 0, len(ids))
	err = r.corpus.write(ctx, zoneIDValue, func(statements statements) error {
		for _, id := range ids {
			row, err := statements.GetStoredDocument(ctx, string(id))
			located, err := restoreReadDocument(documentRecord{
				ID: row.ID, Name: row.Name, MediaType: row.MediaType,
				Size: row.Size, Digest: row.Digest, Location: row.Location,
			}, err, "read selected Document")
			if err != nil {
				return err
			}
			if _, err := statements.SaveZoneDocument(ctx, db2.SaveZoneDocumentParams{
				ZoneID: zoneIDValue, DocumentID: string(located.Document.ID),
			}); err != nil {
				return classifySQLite("associate Zone Document", err)
			}
			result = append(result, located)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *DocumentRepository) Get(ctx context.Context, id document.ID) (document.LocatedDocument, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return document.LocatedDocument{}, err
	}
	row, err := newStatements(r.corpus.database).GetDocument(ctx, db2.GetDocumentParams{
		ZoneID: string(zoneID), ID: string(id),
	})
	return restoreReadDocument(documentRecord{
		ID: row.ID, Name: row.Name, MediaType: row.MediaType,
		Size: row.Size, Digest: row.Digest, Location: row.Location,
	}, err, "read Document")
}

func (r *DocumentRepository) GetByDigest(
	ctx context.Context,
	digest string,
) (document.LocatedDocument, error) {
	if _, err := zone.RequireID(ctx); err != nil {
		return document.LocatedDocument{}, err
	}
	row, err := newStatements(r.corpus.database).GetDocumentByDigest(ctx, digest)
	return restoreReadDocument(documentRecord{
		ID: row.ID, Name: row.Name, MediaType: row.MediaType,
		Size: row.Size, Digest: row.Digest, Location: row.Location,
	}, err, "read Document by Digest")
}

func (r *DocumentRepository) GetByLocation(
	ctx context.Context,
	location document.Location,
) (document.LocatedDocument, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return document.LocatedDocument{}, err
	}
	row, err := newStatements(r.corpus.database).GetDocumentByLocation(ctx, db2.GetDocumentByLocationParams{
		ZoneID: string(zoneID), Location: string(location),
	})
	return restoreReadDocument(documentRecord{
		ID: row.ID, Name: row.Name, MediaType: row.MediaType,
		Size: row.Size, Digest: row.Digest, Location: row.Location,
	}, err, "read Document by Location")
}

func (r *DocumentRepository) List(
	ctx context.Context,
	page document.Page,
) (document.LocatedDocumentPage, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return document.LocatedDocumentPage{}, err
	}
	rows, err := newStatements(r.corpus.database).ListDocuments(ctx, db2.ListDocumentsParams{
		ZoneID: string(zoneID), Limit: int64(page.Limit + 1), Offset: int64(page.Offset),
	})
	if err != nil {
		return document.LocatedDocumentPage{}, classifySQLite("list Documents", err)
	}
	count := min(len(rows), page.Limit)
	values := make([]document.LocatedDocument, count)
	for index := 0; index < count; index++ {
		row := rows[index]
		values[index], err = restoreLocatedDocument(documentRecord{
			ID: row.ID, Name: row.Name, MediaType: row.MediaType,
			Size: row.Size, Digest: row.Digest, Location: row.Location,
		})
		if err != nil {
			return document.LocatedDocumentPage{}, err
		}
	}
	result := document.LocatedDocumentPage{Documents: values}
	if len(rows) > page.Limit {
		result.Next = &document.Page{Offset: page.Offset + page.Limit, Limit: page.Limit}
	}
	return result, nil
}

type documentRecord struct {
	ID        string
	Name      string
	MediaType string
	Size      int64
	Digest    string
	Location  string
}

func restoreReadDocument(row documentRecord, err error, operation string) (document.LocatedDocument, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return document.LocatedDocument{}, document.ErrNotFound
	}
	if err != nil {
		return document.LocatedDocument{}, classifySQLite(operation, err)
	}
	return restoreLocatedDocument(row)
}

func restoreLocatedDocument(row documentRecord) (document.LocatedDocument, error) {
	value, err := restoreDocument(row)
	if err != nil {
		return document.LocatedDocument{}, err
	}
	if row.Location == "" {
		return document.LocatedDocument{}, document.ErrStorageIntegrity
	}
	return document.LocatedDocument{Document: value, Location: document.Location(row.Location)}, nil
}

func restoreDocument(row documentRecord) (document.Document, error) {
	value := document.Document{
		ID: document.ID(row.ID), Name: row.Name, MediaType: row.MediaType,
		Size: row.Size, Digest: row.Digest,
	}
	restored, err := document.Restore(value)
	if err != nil {
		return document.Document{}, fmt.Errorf("%w: %v", document.ErrStorageIntegrity, err)
	}
	return restored, nil
}
