package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sqlitedriver "modernc.org/sqlite"
)

func verifyConnection(ctx context.Context, database *sql.DB) error {
	var vectorVersion string
	if err := database.QueryRowContext(ctx, "SELECT vec_version()").Scan(&vectorVersion); err != nil {
		return fmt.Errorf("verify Semantic sqlite-vec extension: %w", err)
	}
	if vectorVersion != expectedSQLiteVecVersion {
		return fmt.Errorf(
			"Semantic sqlite-vec version is %q; expected %q",
			vectorVersion,
			expectedSQLiteVecVersion,
		)
	}
	var foreignKeys int
	if err := database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("verify Semantic SQLite foreign_keys: %w", err)
	}
	if foreignKeys != 0 {
		return errors.New("Semantic SQLite foreign_keys must be disabled")
	}
	var busyTimeout int
	if err := database.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("verify Semantic SQLite busy_timeout: %w", err)
	}
	if busyTimeout != 5000 {
		return fmt.Errorf(
			"Semantic SQLite busy_timeout is %d milliseconds; expected 5000",
			busyTimeout,
		)
	}
	var synchronous int
	if err := database.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return fmt.Errorf("verify Semantic SQLite synchronous: %w", err)
	}
	if synchronous != 2 {
		return fmt.Errorf("Semantic SQLite synchronous mode is %d; expected FULL", synchronous)
	}
	return nil
}

func classifySQLite(operation string, err error) error {
	if err == nil {
		return nil
	}
	if isBusy(err) {
		return fmt.Errorf("%s: Semantic SQLite is busy: %w", operation, err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func isBusy(err error) bool {
	var sqliteError *sqlitedriver.Error
	return errors.As(err, &sqliteError) &&
		(sqliteError.Code()&0xff == 5 || sqliteError.Code()&0xff == 6)
}
