package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/epoch"
)

// Store maps Epoch persistence onto a caller-owned SQLite connection pool. It
// initializes and verifies only Epoch schema and never owns the pool lifecycle
// or a Project path.
type Store struct {
	// database is the fact source for immutable Epoch rows and Current selection.
	// The caller keeps it open until every Store operation has completed.
	database *sql.DB
}

var _ epoch.Store = (*Store)(nil)

// NewStore binds Epoch persistence to an available, preconfigured SQLite pool.
// Empty databases receive the current schema; unsupported versions are rejected.
// The supplied pool remains owned by the caller on success and failure.
func NewStore(database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("create Epoch SQLite Store: database is required")
	}
	ctx := context.Background()
	if err := database.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect Epoch SQLite: %w", err)
	}
	if err := verifyConnection(ctx, database); err != nil {
		return nil, err
	}
	if err := enableWAL(database); err != nil {
		return nil, err
	}
	if err := initialize(database); err != nil {
		return nil, err
	}
	return &Store{database: database}, nil
}

const (
	walEnableRetryWindow   = 5 * time.Second
	walEnableRetryInterval = 10 * time.Millisecond
)

func enableWAL(database *sql.DB) error {
	deadline := time.Now().Add(walEnableRetryWindow)
	for {
		var mode string
		err := database.QueryRow("PRAGMA journal_mode = WAL").Scan(&mode)
		if err == nil {
			if mode != "wal" {
				return fmt.Errorf(
					"%w: Epoch SQLite journal mode is %q; expected wal",
					epoch.ErrEpochDataIntegrity,
					mode,
				)
			}
			return nil
		}
		classified := classifySQLite("enable Epoch SQLite WAL", err)
		if !errors.Is(classified, epoch.ErrEpochStorageBusy) ||
			!time.Now().Before(deadline) {
			return classified
		}
		time.Sleep(walEnableRetryInterval)
	}
}
