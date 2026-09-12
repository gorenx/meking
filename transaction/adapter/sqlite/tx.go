// Package sqlite implements the shared SQLite transaction scope used by
// Project-assembled application services.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNoTransaction     = errors.New("no active SQLite transaction")
	ErrDifferentDatabase = errors.New("active transaction belongs to another SQLite database")
)

// Executor is the SQL surface available to persistence adapters participating
// in the current transaction.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Tx owns transaction lifetime for one caller-owned SQLite pool.
type Tx struct {
	database *sql.DB
	writes   sync.Mutex
}

type contextKey struct{}

type activeTransaction struct {
	owner    *Tx
	executor Executor
}

func New(database *sql.DB) (*Tx, error) {
	if database == nil {
		return nil, errors.New("create SQLite transaction scope: database is required")
	}
	return &Tx{database: database}, nil
}

func (t *Tx) WithTx(
	ctx context.Context,
	work func(ctx context.Context) error,
) (resultErr error) {
	if t == nil || t.database == nil {
		return errors.New("execute SQLite transaction: scope is not configured")
	}
	if work == nil {
		return errors.New("execute SQLite transaction: callback is required")
	}
	if current, ok := ctx.Value(contextKey{}).(*activeTransaction); ok && current != nil {
		if current.owner == nil || current.owner.database != t.database {
			return ErrDifferentDatabase
		}
		return work(ctx)
	}
	t.writes.Lock()
	defer t.writes.Unlock()
	connection, err := t.database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve SQLite write connection: %w", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release SQLite write connection: %w", closeErr))
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate SQLite transaction: %w", err)
	}
	open := true
	defer func() {
		if !open {
			return
		}
		if _, rollbackErr := connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK"); rollbackErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("roll back SQLite transaction: %w", rollbackErr))
		}
	}()
	transactionContext := context.WithValue(ctx, contextKey{}, &activeTransaction{
		owner:    t,
		executor: connection,
	})
	if err := work(transactionContext); err != nil {
		return err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit SQLite transaction: %w", err)
	}
	open = false
	return nil
}

// Current returns the active transaction for database. Persistence adapters
// use this only for writes that must participate in an application transaction.
func Current(ctx context.Context, database *sql.DB) (Executor, error) {
	if database == nil {
		return nil, errors.New("resolve SQLite transaction: database is required")
	}
	active, ok := ctx.Value(contextKey{}).(*activeTransaction)
	if !ok || active == nil || active.executor == nil {
		return nil, ErrNoTransaction
	}
	if active.owner == nil || active.owner.database != database {
		return nil, ErrDifferentDatabase
	}
	return active.executor, nil
}
