// Package sqlite implements Journal-owned persistence. It joins an active
// transaction through context and does not access any domain table.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/journal/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/transaction"
)

const schemaVersion = 5

// schema is the exact DDL used by runtime initialization and sqlc generation.
//
//go:embed schema.sql
var schema string

type Store struct {
	database     *sql.DB
	transactions transaction.Tx
	queries      *db.Queries
}

var (
	_ journal.EventAppender         = (*Store)(nil)
	_ journal.EventReader           = (*Store)(nil)
	_ journal.PendingEventReader    = (*Store)(nil)
	_ journal.ConsumerPositions      = (*Store)(nil)
)

func NewStore(database *sql.DB, transactions transaction.Tx) (*Store, error) {
	if database == nil {
		return nil, errors.New("create Journal SQLite Store: database is required")
	}
	if transactions == nil {
		return nil, errors.New("create Journal SQLite Store: transactions are required")
	}
	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("connect Journal SQLite: %w", err)
	}
	if err := initialize(database); err != nil {
		return nil, err
	}
	return &Store{
		database: database, transactions: transactions, queries: db.New(database),
	}, nil
}

func initialize(database *sql.DB) (resultErr error) {
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve Journal SQLite initialization connection: %w", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release Journal SQLite initialization connection: %w", closeErr))
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin Journal SQLite initialization: %w", err)
	}
	transactionOpen := true
	defer func() {
		if !transactionOpen {
			return
		}
		if _, rollbackErr := connection.ExecContext(ctx, "ROLLBACK"); rollbackErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("roll back Journal SQLite initialization: %w", rollbackErr))
		}
	}()

	queries := db.New(connection)
	version, err := queries.GetSchemaVersion(ctx)
	switch {
	case err == nil:
		if version != schemaVersion {
			return fmt.Errorf(
				"open Journal SQLite: unsupported schema version %d; expected %d",
				version,
				schemaVersion,
			)
		}
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("open Journal SQLite: schema version row is missing")
	default:
		versionReadErr := err
		if _, err := connection.ExecContext(ctx, schema); err != nil {
			return errors.Join(
				fmt.Errorf("read Journal SQLite schema version: %w", versionReadErr),
				fmt.Errorf("initialize Journal SQLite schema: %w", err),
			)
		}
		queries = db.New(connection)
	}
	if _, err := queries.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("validate Journal SQLite schema: %w", err)
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit Journal SQLite initialization: %w", err)
	}
	transactionOpen = false
	return nil
}
