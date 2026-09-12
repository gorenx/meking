package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/epoch"
)

const schemaVersion = 11

// schema is the exact DDL used by runtime initialization and sqlc generation.
//
//go:embed schema.sql
var schema string

func initialize(database *sql.DB) (resultErr error) {
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		return classifySQLite("reserve Epoch SQLite initialization connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("release Epoch SQLite initialization connection", closeErr),
			)
		}
	}()

	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Epoch SQLite initialization", err)
	}
	transactionOpen := true
	defer func() {
		if !transactionOpen {
			return
		}
		if _, rollbackErr := connection.ExecContext(ctx, "ROLLBACK"); rollbackErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("roll back Epoch SQLite initialization", rollbackErr),
			)
		}
	}()

	queries := newGlobalStatements(connection)
	version, err := queries.GetSchemaVersion(ctx)
	switch {
	case err == nil:
		if version != schemaVersion {
			return fmt.Errorf(
				"open Epoch SQLite: unsupported schema version %d; expected %d",
				version,
				schemaVersion,
			)
		}
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: Epoch schema version row is missing", epoch.ErrEpochDataIntegrity)
	default:
		versionReadErr := err
		if _, err := connection.ExecContext(ctx, schema); err != nil {
			return errors.Join(
				fmt.Errorf("read Epoch SQLite schema version: %w", versionReadErr),
				classifySQLite("initialize Epoch SQLite schema", err),
			)
		}
		queries = newGlobalStatements(connection)
	}
	if _, err := queries.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("%w: validate Epoch SQLite schema: %v", epoch.ErrEpochDataIntegrity, err)
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Epoch SQLite initialization", err)
	}
	transactionOpen = false
	return nil
}
