package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	db2 "github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/zone"
)

type TextStore struct {
	corpus *Store
}

var _ text.Store = (*TextStore)(nil)

func NewTextStore(store *Store) (*TextStore, error) {
	if store == nil || store.database == nil {
		return nil, errors.New("create Text SQLite Store: Corpus Store is required")
	}
	return &TextStore{corpus: store}, nil
}

func (s *TextStore) Save(ctx context.Context, value text.Text) (text.Text, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return text.Text{}, err
	}
	zoneIDValue := string(zoneID)
	var saved text.Text
	err = s.corpus.write(ctx, zoneIDValue, func(statements statements) error {
		if _, err := statements.GetDocument(ctx,
			db2.GetDocumentParams{
				ZoneID: zoneIDValue, ID: string(value.DocumentID),
			}); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: Document %q does not exist", text.ErrInvalid, value.DocumentID)
		} else if err != nil {
			return classifySQLite("read Text Document", err)
		}
		var err error
		saved, err = saveText(ctx, statements, zoneIDValue, value)
		if err != nil {
			return err
		}
		if err := statements.MarkLocalText(ctx, db2.MarkLocalTextParams{
			ZoneID: zoneIDValue, TextID: string(saved.ID),
		}); err != nil {
			return classifySQLite("record local Text", err)
		}
		return nil
	})
	return saved, err
}

func saveText(ctx context.Context, statements statements, zoneID string, value text.Text) (text.Text, error) {

	inserted, err := statements.SaveText(ctx, db2.SaveTextParams{
		ID:                   string(value.ID),
		SourceDocumentID:     string(value.DocumentID),
		Title:                value.Title,
		Body:                 value.Body,
		Format:               string(value.Format),
		ExtractionProfile:    value.ExtractionProfile,
		NormalizationProfile: value.NormalizationProfile,
	})
	if err != nil {
		return text.Text{}, classifySQLite("save Text", err)
	}
	if inserted == 0 {
		existing, err := loadStoredText(ctx, statements, zoneID, value.ID)
		if err == nil {
			if !equalText(existing, value) {
				return text.Text{}, text.ErrContentConflict
			}
			return existing, nil
		}
		if !errors.Is(err, text.ErrNotFound) {
			return text.Text{}, err
		}
		existing, err = findStoredText(
			ctx,
			statements,
			zoneID,
			value.DocumentID,
			value.ExtractionProfile,
			value.NormalizationProfile,
		)
		if err != nil {
			return text.Text{}, err
		}
		if !equalTextContent(existing, value) {
			return text.Text{}, text.ErrContentConflict
		}
		return existing, nil
	}
	for ordinal, warning := range value.Warnings {
		if err := statements.AddTextWarning(ctx, db2.AddTextWarningParams{
			TextID: string(value.ID), WarningPosition: int64(ordinal), Warning: warning,
		}); err != nil {
			return text.Text{}, classifySQLite("add Text warning", err)
		}
	}
	return value, nil
}

func (s *TextStore) Get(ctx context.Context, id text.ID) (text.Text, error) {
	statements, zoneID, err := s.corpus.reader(ctx)
	if err != nil {
		return text.Text{}, err
	}
	return loadText(ctx, statements, zoneID, id)
}

func (s *TextStore) Find(
	ctx context.Context,
	documentID document.ID,
	extractionProfile string,
	normalizationProfile string,
) (text.Text, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return text.Text{}, err
	}
	statements := newStatements(s.corpus.database)
	return findText(ctx, statements, string(zoneID), documentID, extractionProfile, normalizationProfile)
}

func findText(
	ctx context.Context,
	statements statements,
	zoneID string,
	documentID document.ID,
	extractionProfile string,
	normalizationProfile string,
) (text.Text, error) {
	row, err := statements.FindText(ctx, db2.FindTextParams{
		ZoneID:               zoneID,
		SourceDocumentID:     string(documentID),
		ExtractionProfile:    extractionProfile,
		NormalizationProfile: normalizationProfile,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return text.Text{}, text.ErrNotFound
	}
	if err != nil {
		return text.Text{}, classifySQLite("find Text", err)
	}
	warningRows, err := statements.ListTextWarnings(ctx, row.ID)
	if err != nil {
		return text.Text{}, classifySQLite("list Text warnings", err)
	}
	warnings := make([]string, len(warningRows))
	for position, warning := range warningRows {
		if warning.TextID != row.ID || warning.WarningPosition != int64(position) {
			return text.Text{}, fmt.Errorf("%w: Text %q warnings are not contiguous", text.ErrStorageIntegrity, row.ID)
		}
		warnings[position] = warning.Warning
	}
	return restoreText(textRecord{
		ID: row.ID, SourceDocumentID: row.SourceDocumentID, Title: row.Title,
		Body: row.Body, Format: row.Format, ExtractionProfile: row.ExtractionProfile,
		NormalizationProfile: row.NormalizationProfile,
	}, warnings)
}

func loadText(ctx context.Context, statements statements, zoneID string, id text.ID) (text.Text, error) {
	row, err := statements.GetText(ctx, db2.GetTextParams{ZoneID: zoneID, ID: string(id)})
	if errors.Is(err, sql.ErrNoRows) {
		return text.Text{}, text.ErrNotFound
	}
	if err != nil {
		return text.Text{}, classifySQLite("read Text", err)
	}
	if _, err := statements.GetDocument(ctx, db2.GetDocumentParams{
		ZoneID: zoneID, ID: row.SourceDocumentID,
	}); errors.Is(err, sql.ErrNoRows) {
		return text.Text{}, fmt.Errorf("%w: Text %q references missing Document %q", text.ErrStorageIntegrity, id, row.SourceDocumentID)
	} else if err != nil {
		return text.Text{}, classifySQLite("read Text Document", err)
	}
	warningRows, err := statements.ListTextWarnings(ctx, string(id))
	if err != nil {
		return text.Text{}, classifySQLite("list Text warnings", err)
	}
	warnings := make([]string, len(warningRows))
	for position, warning := range warningRows {
		if warning.TextID != string(id) || warning.WarningPosition != int64(position) {
			return text.Text{}, fmt.Errorf("%w: Text %q warnings are not contiguous", text.ErrStorageIntegrity, id)
		}
		warnings[position] = warning.Warning
	}
	return restoreText(textRecord{
		ID: row.ID, SourceDocumentID: row.SourceDocumentID, Title: row.Title,
		Body: row.Body, Format: row.Format, ExtractionProfile: row.ExtractionProfile,
		NormalizationProfile: row.NormalizationProfile,
	}, warnings)
}

func loadStoredText(ctx context.Context, statements statements, zoneID string, id text.ID) (text.Text, error) {
	row, err := statements.GetStoredText(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return text.Text{}, text.ErrNotFound
	}
	if err != nil {
		return text.Text{}, classifySQLite("read stored Text", err)
	}
	if _, err := statements.GetDocument(ctx, db2.GetDocumentParams{
		ZoneID: zoneID, ID: row.SourceDocumentID,
	}); errors.Is(err, sql.ErrNoRows) {
		return text.Text{}, fmt.Errorf("%w: Text %q references missing Document %q", text.ErrStorageIntegrity, id, row.SourceDocumentID)
	} else if err != nil {
		return text.Text{}, classifySQLite("read stored Text Document", err)
	}
	warningRows, err := statements.ListTextWarnings(ctx, string(id))
	if err != nil {
		return text.Text{}, classifySQLite("list stored Text warnings", err)
	}
	warnings := make([]string, len(warningRows))
	for position, warning := range warningRows {
		if warning.TextID != string(id) || warning.WarningPosition != int64(position) {
			return text.Text{}, fmt.Errorf("%w: Text %q warnings are not contiguous", text.ErrStorageIntegrity, id)
		}
		warnings[position] = warning.Warning
	}
	return restoreText(textRecord{
		ID: row.ID, SourceDocumentID: row.SourceDocumentID, Title: row.Title,
		Body: row.Body, Format: row.Format, ExtractionProfile: row.ExtractionProfile,
		NormalizationProfile: row.NormalizationProfile,
	}, warnings)
}

func findStoredText(
	ctx context.Context,
	statements statements,
	zoneID string,
	documentID document.ID,
	extractionProfile string,
	normalizationProfile string,
) (text.Text, error) {
	row, err := statements.FindStoredText(ctx, db2.FindStoredTextParams{
		SourceDocumentID:     string(documentID),
		ExtractionProfile:    extractionProfile,
		NormalizationProfile: normalizationProfile,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return text.Text{}, text.ErrNotFound
	}
	if err != nil {
		return text.Text{}, classifySQLite("find stored Text", err)
	}
	return loadStoredText(ctx, statements, zoneID, text.ID(row.ID))
}

func (s *TextStore) List(ctx context.Context, page text.Page) (text.TextPage, error) {
	if page.Offset < 0 || page.Limit <= 0 || page.Limit > 1000 {
		return text.TextPage{}, text.ErrInvalid
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return text.TextPage{}, err
	}
	statements := newStatements(s.corpus.database)
	rows, err := statements.ListTexts(ctx, db2.ListTextsParams{
		ZoneID: string(zoneID), Limit: int64(page.Limit + 1), Offset: int64(page.Offset),
	})
	if err != nil {
		return text.TextPage{}, classifySQLite("list Texts", err)
	}
	count := min(len(rows), page.Limit)
	values := make([]text.Text, count)
	for index := 0; index < count; index++ {
		if _, err := statements.GetDocument(ctx, db2.GetDocumentParams{
			ZoneID: string(zoneID), ID: rows[index].SourceDocumentID,
		}); errors.Is(err, sql.ErrNoRows) {
			return text.TextPage{}, fmt.Errorf(
				"%w: Text %q references missing Document %q",
				text.ErrStorageIntegrity,
				rows[index].ID,
				rows[index].SourceDocumentID,
			)
		} else if err != nil {
			return text.TextPage{}, classifySQLite("read Text Document", err)
		}
		warningRows, err := statements.ListTextWarnings(ctx, rows[index].ID)
		if err != nil {
			return text.TextPage{}, classifySQLite("list Text warnings", err)
		}
		warnings := make([]string, len(warningRows))
		for position, warning := range warningRows {
			if warning.TextID != rows[index].ID || warning.WarningPosition != int64(position) {
				return text.TextPage{}, fmt.Errorf("%w: Text %q warnings are not contiguous", text.ErrStorageIntegrity, rows[index].ID)
			}
			warnings[position] = warning.Warning
		}
		row := rows[index]
		values[index], err = restoreText(textRecord{
			ID: row.ID, SourceDocumentID: row.SourceDocumentID, Title: row.Title,
			Body: row.Body, Format: row.Format, ExtractionProfile: row.ExtractionProfile,
			NormalizationProfile: row.NormalizationProfile,
		}, warnings)
		if err != nil {
			return text.TextPage{}, err
		}
	}
	result := text.TextPage{Texts: values}
	if len(rows) > page.Limit {
		next := text.Page{Offset: page.Offset + page.Limit, Limit: page.Limit}
		result.Next = &next
	}
	return result, nil
}

type textRecord struct {
	ID                   string
	SourceDocumentID     string
	Title                string
	Body                 string
	Format               string
	ExtractionProfile    string
	NormalizationProfile string
}

func restoreText(row textRecord, warnings []string) (text.Text, error) {
	value := text.Text{
		ID:                   text.ID(row.ID),
		DocumentID:           document.ID(row.SourceDocumentID),
		Title:                row.Title,
		Body:                 row.Body,
		Format:               text.Format(row.Format),
		ExtractionProfile:    row.ExtractionProfile,
		NormalizationProfile: row.NormalizationProfile,
		Warnings:             append([]string(nil), warnings...),
	}
	if err := text.Validate(value); err != nil {
		return text.Text{}, fmt.Errorf("%w: %v", text.ErrStorageIntegrity, err)
	}
	return value, nil
}

func equalText(left, right text.Text) bool {
	return left.ID == right.ID && left.DocumentID == right.DocumentID &&
		left.Title == right.Title && left.Body == right.Body && left.Format == right.Format &&
		left.ExtractionProfile == right.ExtractionProfile &&
		left.NormalizationProfile == right.NormalizationProfile &&
		slices.Equal(left.Warnings, right.Warnings)
}

func equalTextContent(left, right text.Text) bool {
	left.ID = ""
	right.ID = ""
	return equalText(left, right)
}
