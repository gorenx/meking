// Package sqlite stores Zone-scoped recall observations in the shared Project database.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/memory/activation"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
	sqlitedriver "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Database struct{ database *sql.DB }

func New(database *sql.DB) (*Database, error) {
	if database == nil {
		return nil, errors.New("activation: SQLite database is required")
	}
	tx, err := transactionsqlite.New(database)
	if err != nil {
		return nil, err
	}
	result := &Database{database: database}
	err = tx.WithTx(context.Background(), func(ctx context.Context) error {
		executor, err := transactionsqlite.Current(ctx, database)
		if err != nil {
			return err
		}
		var count int
		if err := executor.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='activation_schema'").Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := executor.ExecContext(ctx, schema); err != nil {
				return err
			}
		}
		var version int
		if err := executor.QueryRowContext(ctx, "SELECT version FROM activation_schema WHERE id=1").Scan(&version); err != nil {
			return err
		}
		if version != 1 {
			return fmt.Errorf("%w: unsupported schema version %d", activation.ErrDataIntegrity, version)
		}
		return nil
	})
	if err != nil {
		return nil, classify(err)
	}
	return result, nil
}

func (database *Database) reader(ctx context.Context) (transactionsqlite.Executor, error) {
	executor, err := transactionsqlite.Current(ctx, database.database)
	if errors.Is(err, transactionsqlite.ErrNoTransaction) {
		return database.database, nil
	}
	return executor, err
}

func (database *Database) scopedReader(ctx context.Context) (transactionsqlite.Executor, string, error) {
	id, err := zone.RequireID(ctx)
	if err != nil {
		return nil, "", err
	}
	executor, err := database.reader(ctx)
	return executor, string(id), err
}

func (database *Database) scopedWriter(ctx context.Context) (transactionsqlite.Executor, string, error) {
	id, err := zone.RequireID(ctx)
	if err != nil {
		return nil, "", err
	}
	executor, err := transactionsqlite.Current(ctx, database.database)
	return executor, string(id), err
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	var sqliteErr *sqlitedriver.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() & 0xff {
		case 5, 6:
			return fmt.Errorf("%w: %v", activation.ErrStorageUnavailable, err)
		case 11, 19, 26:
			return fmt.Errorf("%w: %v", activation.ErrDataIntegrity, err)
		}
	}
	return fmt.Errorf("activation storage: %w", err)
}
