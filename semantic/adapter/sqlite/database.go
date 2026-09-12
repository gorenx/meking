package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/semantic/adapter/sqlite/internal/db"
)

func vectorDatabase(ctx context.Context, statements statements) (db.GetVectorDatabaseRow, error) {
	configured, err := statements.GetVectorDatabase(ctx)
	if err != nil {
		return db.GetVectorDatabaseRow{}, classifySQLite(
			"read Semantic database configuration", err,
		)
	}
	if configured.Model == "" || configured.Dimension < 0 {
		return db.GetVectorDatabaseRow{}, errors.New("Semantic database configuration is invalid")
	}
	return configured, nil
}

func fixDimension(
	ctx context.Context,
	statements statements,
	proposed int,
) (db.GetVectorDatabaseRow, error) {
	configured, err := vectorDatabase(ctx, statements)
	if err != nil {
		return db.GetVectorDatabaseRow{}, err
	}
	if proposed < 0 {
		return db.GetVectorDatabaseRow{}, errors.New("Semantic vector dimension is negative")
	}
	if proposed == 0 {
		return configured, nil
	}
	if configured.Dimension == 0 {
		if err = statements.SetVectorDimension(ctx, int64(proposed)); err != nil {
			return db.GetVectorDatabaseRow{}, classifySQLite(
				"fix Semantic database dimension", err,
			)
		}
		configured.Dimension = int64(proposed)
	}
	if configured.Dimension != int64(proposed) {
		return db.GetVectorDatabaseRow{}, fmt.Errorf(
			"Semantic vector dimension %d does not match database dimension %d",
			proposed, configured.Dimension,
		)
	}
	return configured, nil
}
