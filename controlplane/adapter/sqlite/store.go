// Package sqlite implements Control Plane Policy persistence.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

type Store struct {
	database *sql.DB
}

func NewStore(database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("create Control Plane SQLite Store: database is required")
	}
	if err := database.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("connect Control Plane SQLite: %w", err)
	}
	if err := initialize(database); err != nil {
		return nil, err
	}
	return &Store{
		database: database,
	}, nil
}

func (store *Store) reader(ctx context.Context) (transactionsqlite.Executor, error) {
	executor, err := transactionsqlite.Current(ctx, store.database)
	if err == nil {
		return executor, nil
	}
	if errors.Is(err, transactionsqlite.ErrNoTransaction) {
		return store.database, nil
	}
	return nil, fmt.Errorf("join Control Plane read transaction: %w", err)
}

func (store *Store) writer(ctx context.Context) (transactionsqlite.Executor, error) {
	executor, err := transactionsqlite.Current(ctx, store.database)
	if err != nil {
		return nil, fmt.Errorf("join Control Plane transaction: %w", err)
	}
	return executor, nil
}
