package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
)

const schemaVersion = 4

//go:embed schema.sql
var schema string

func initialize(database *sql.DB) (resultErr error) {
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve Control Plane SQLite initialization connection: %w", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("release Control Plane SQLite initialization connection: %w", closeErr),
			)
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin Control Plane SQLite initialization: %w", err)
	}
	open := true
	defer func() {
		if !open {
			return
		}
		if _, rollbackErr := connection.ExecContext(ctx, "ROLLBACK"); rollbackErr != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("roll back Control Plane SQLite initialization: %w", rollbackErr),
			)
		}
	}()
	var version int
	err = connection.QueryRowContext(
		ctx,
		"SELECT version FROM controlplane_schema WHERE id = 1",
	).Scan(&version)
	switch {
	case err == nil:
		if version != schemaVersion {
			return fmt.Errorf(
				"open Control Plane SQLite: unsupported schema version %d; expected %d",
				version,
				schemaVersion,
			)
		}
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("open Control Plane SQLite: schema version row is missing")
	default:
		versionReadErr := err
		if _, err := connection.ExecContext(ctx, schema); err != nil {
			return errors.Join(
				fmt.Errorf("read Control Plane SQLite schema version: %w", versionReadErr),
				fmt.Errorf("initialize Control Plane SQLite schema: %w", err),
			)
		}
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit Control Plane SQLite initialization: %w", err)
	}
	open = false
	return nil
}
