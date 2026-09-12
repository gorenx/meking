package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/knowledge/extraction"
)

func (database *Database) ExtractionProgress(ctx context.Context, corporaID string) (string, bool, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return "", false, err
	}
	last, err := queries.GetKnowledgeExtractionProgress(ctx, db.GetKnowledgeExtractionProgressParams{
		ZoneID:    queries.zoneID,
		CorporaID: corporaID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, classifySQLite("read Knowledge Extraction progress", err)
	}
	return last, true, nil
}

func (database *Database) AdvanceExtraction(
	ctx context.Context,
	corporaID string,
	expectedTextUnitID string,
	lastTextUnitID string,
) error {
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	if expectedTextUnitID == "" {
		err := queries.StartKnowledgeExtractionProgress(ctx, db.StartKnowledgeExtractionProgressParams{
			ZoneID:         queries.zoneID,
			CorporaID:      corporaID,
			LastTextUnitID: lastTextUnitID,
		})
		if constraint(err) {
			return extraction.ErrExtractionProgress
		}
		return classifySQLite("start Knowledge Extraction progress", err)
	}
	count, err := queries.AdvanceKnowledgeExtractionProgress(ctx, db.AdvanceKnowledgeExtractionProgressParams{
		LastTextUnitID:     lastTextUnitID,
		ZoneID:             queries.zoneID,
		CorporaID:          corporaID,
		ExpectedTextUnitID: expectedTextUnitID,
	})
	if err != nil {
		return classifySQLite("advance Knowledge Extraction progress", err)
	}
	if count != 1 {
		return fmt.Errorf("%w: expected last TextUnit %q", extraction.ErrExtractionProgress, expectedTextUnitID)
	}
	return nil
}
