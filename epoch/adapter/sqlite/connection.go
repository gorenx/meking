package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/epoch"
	sqlitedriver "modernc.org/sqlite"
)

func verifyConnection(ctx context.Context, database *sql.DB) error {
	var foreignKeys int
	if err := database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("verify Epoch SQLite foreign_keys: %w", err)
	}
	if foreignKeys != 0 {
		return fmt.Errorf(
			"%w: Epoch SQLite foreign_keys must be disabled",
			epoch.ErrEpochDataIntegrity,
		)
	}
	var busyTimeout int
	if err := database.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("verify Epoch SQLite busy_timeout: %w", err)
	}
	if busyTimeout != 5000 {
		return fmt.Errorf(
			"%w: Epoch SQLite busy_timeout is %d milliseconds; expected 5000",
			epoch.ErrEpochDataIntegrity,
			busyTimeout,
		)
	}
	var synchronous int
	if err := database.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return fmt.Errorf("verify Epoch SQLite synchronous: %w", err)
	}
	if synchronous != 2 {
		return fmt.Errorf(
			"%w: Epoch SQLite synchronous mode is %d; expected FULL",
			epoch.ErrEpochDataIntegrity,
			synchronous,
		)
	}
	return nil
}

func classifySQLite(operation string, err error) error {
	if err == nil {
		return nil
	}
	var sqliteError *sqlitedriver.Error
	if errors.As(err, &sqliteError) {
		switch sqliteError.Code() & 0xff {
		case 5, 6:
			return fmt.Errorf("%s: %w: %v", operation, epoch.ErrEpochStorageBusy, err)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
