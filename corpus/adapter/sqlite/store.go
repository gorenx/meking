// Package sqlite persists immutable Corpus content, ordered Corpora, and the
// current Corpora selection in a caller-owned SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	sqlitedriver "modernc.org/sqlite"
)

const (
	walEnableRetryWindow   = 5 * time.Second
	walEnableRetryInterval = 10 * time.Millisecond
)

// Store maps the Corpus persistence port onto a caller-owned SQLite connection
// pool. It initializes and verifies only the Corpus schema and never owns the
// pool lifecycle or a Project path.
type Store struct {
	// database is the fact source for immutable content, ordered occurrences,
	// Corpora settings, and current selection. The caller keeps it open until
	// every Store operation has completed.
	database *sql.DB
}

var _ corpus.Store = (*Store)(nil)

// NewStore binds Corpus persistence to an available, preconfigured SQLite pool.
// Databases without Corpus objects receive the current module schema. Existing
// Corpus schemas must already match this adapter's exact version;
// schema migration belongs to an explicit deployment operation.
// The supplied pool remains owned by the caller on success and failure.
func NewStore(database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("create Corpus SQLite Store: database is required")
	}
	ctx := context.Background()
	if err := database.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect Corpus SQLite: %w", err)
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

func (s *Store) DocumentRepository() document.Repository {
	return &DocumentRepository{corpus: s}
}

func (s *Store) TextStore() text.Store {
	return &TextStore{corpus: s}
}

func enableWAL(database *sql.DB) error {
	deadline := time.Now().Add(walEnableRetryWindow)
	for {
		var mode string
		err := database.QueryRow("PRAGMA journal_mode = WAL").Scan(&mode)
		if err == nil {
			if mode != "wal" {
				return fmt.Errorf(
					"%w: Corpus SQLite journal mode is %q; expected wal",
					corpus.ErrCorpusDataIntegrity,
					mode,
				)
			}
			return nil
		}
		classified := classifySQLite("enable Corpus SQLite WAL", err)
		if !errors.Is(classified, corpus.ErrCorpusStorageBusy) ||
			!time.Now().Before(deadline) {
			return classified
		}
		time.Sleep(walEnableRetryInterval)
	}
}

func initialize(database *sql.DB) (resultErr error) {
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		return classifySQLite("reserve Corpus SQLite initialization connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("release Corpus SQLite initialization connection", closeErr),
			)
		}
	}()

	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Corpus SQLite initialization", err)
	}
	transactionOpen := true
	defer func() {
		if !transactionOpen {
			return
		}
		_, rollbackErr := connection.ExecContext(ctx, "ROLLBACK")
		if rollbackErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("roll back Corpus SQLite initialization", rollbackErr),
			)
		}
	}()

	queries := newStatements(connection)
	version, err := queries.GetSchemaVersion(ctx)
	switch {
	case err == nil:
		if version != schemaVersion {
			return fmt.Errorf(
				"open Corpus SQLite: unsupported schema version %d; expected %d",
				version,
				schemaVersion,
			)
		}
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: Corpus schema version row is missing", corpus.ErrCorpusDataIntegrity)
	default:
		versionReadErr := err
		if _, err := connection.ExecContext(ctx, schema); err != nil {
			return errors.Join(
				fmt.Errorf("read Corpus SQLite schema version: %w", versionReadErr),
				classifySQLite("initialize Corpus SQLite schema", err),
			)
		}
		queries = newStatements(connection)
	}
	if _, err := queries.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("%w: validate Corpus SQLite schema: %v", corpus.ErrCorpusDataIntegrity, err)
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Corpus SQLite initialization", err)
	}
	transactionOpen = false
	return nil
}

func verifyConnection(ctx context.Context, database *sql.DB) error {
	var foreignKeys int
	if err := database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("verify Corpus SQLite foreign_keys: %w", err)
	}
	if foreignKeys != 0 {
		return fmt.Errorf(
			"%w: Corpus SQLite foreign_keys must be disabled",
			corpus.ErrCorpusDataIntegrity,
		)
	}
	var busyTimeout int
	if err := database.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("verify Corpus SQLite busy_timeout: %w", err)
	}
	if busyTimeout != 5000 {
		return fmt.Errorf(
			"%w: Corpus SQLite busy_timeout is %d milliseconds; expected 5000",
			corpus.ErrCorpusDataIntegrity,
			busyTimeout,
		)
	}
	var synchronous int
	if err := database.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return fmt.Errorf("verify Corpus SQLite synchronous: %w", err)
	}
	if synchronous != 2 {
		return fmt.Errorf(
			"%w: Corpus SQLite synchronous mode is %d; expected FULL",
			corpus.ErrCorpusDataIntegrity,
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
			return fmt.Errorf("%s: %w: %v", operation, corpus.ErrCorpusStorageBusy, err)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
