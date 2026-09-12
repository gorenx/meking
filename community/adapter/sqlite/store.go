package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/community"
	sqlitedriver "modernc.org/sqlite"
)

const (
	walEnableRetryWindow   = 5 * time.Second
	walEnableRetryInterval = 10 * time.Millisecond
)

// Store maps CommunitySet, Report, and ReportSet persistence ports onto a
// caller-owned SQLite connection pool. It initializes and verifies only the
// Community schema and never owns the pool lifecycle or a Project path.
type Store struct {
	// database is the fact source for immutable Community Structures,
	// CommunitySets, Reports, ReportSets, and their exact references. The caller
	// keeps it open until every Store operation has completed.
	database *sql.DB
}

// NewStore binds Community persistence to an available, preconfigured SQLite
// pool. Databases without Community objects receive the current module schema;
// unrelated Project module objects are ignored. Because Community
// persistence is an unreleased V3 contract, an existing non-current Community
// schema is rejected and must be rebuilt by the owning deployment workflow.
// The supplied pool remains owned by the caller on success and failure.
func NewStore(database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("create Community SQLite Store: database is required")
	}
	ctx := context.Background()
	if err := database.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect Community SQLite: %w", err)
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

func enableWAL(database *sql.DB) error {
	deadline := time.Now().Add(walEnableRetryWindow)
	for {
		var mode string
		err := database.QueryRow("PRAGMA journal_mode = WAL").Scan(&mode)
		if err == nil {
			if mode != "wal" {
				return fmt.Errorf(
					"%w: Community SQLite journal mode is %q; expected wal",
					community.ErrCommunityDataIntegrity,
					mode,
				)
			}
			return nil
		}
		classified := classifySQLite("enable Community SQLite WAL", err)
		if !errors.Is(classified, community.ErrCommunityStorageBusy) ||
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
		return classifySQLite("reserve Community SQLite initialization connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("release Community SQLite initialization connection", closeErr),
			)
		}
	}()

	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Community SQLite initialization", err)
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
				classifySQLite("roll back Community SQLite initialization", rollbackErr),
			)
		}
	}()

	queries := newGlobalStatements(connection)
	version, err := queries.GetSchemaVersion(ctx)
	switch {
	case err == nil:
		if version != schemaVersion {
			return fmt.Errorf(
				"open Community SQLite: unsupported schema version %d; expected %d",
				version,
				schemaVersion,
			)
		}
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: Community schema version row is missing", community.ErrCommunityDataIntegrity)
	default:
		versionReadErr := err
		if _, err := connection.ExecContext(ctx, schema); err != nil {
			return errors.Join(
				fmt.Errorf("read Community SQLite schema version: %w", versionReadErr),
				classifySQLite("initialize Community SQLite schema", err),
			)
		}
		queries = newGlobalStatements(connection)
	}
	if _, err := queries.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("%w: validate Community SQLite schema: %v", community.ErrCommunityDataIntegrity, err)
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Community SQLite initialization", err)
	}
	transactionOpen = false
	return nil
}

func verifyConnection(ctx context.Context, database *sql.DB) error {
	var foreignKeys int
	if err := database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("verify Community SQLite foreign_keys: %w", err)
	}
	if foreignKeys != 0 {
		return fmt.Errorf(
			"%w: Community SQLite foreign_keys must be disabled",
			community.ErrCommunityDataIntegrity,
		)
	}
	var busyTimeout int
	if err := database.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("verify Community SQLite busy_timeout: %w", err)
	}
	if busyTimeout != 5000 {
		return fmt.Errorf(
			"%w: Community SQLite busy_timeout is %d milliseconds; expected 5000",
			community.ErrCommunityDataIntegrity,
			busyTimeout,
		)
	}
	var synchronous int
	if err := database.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return fmt.Errorf("verify Community SQLite synchronous: %w", err)
	}
	if synchronous != 2 {
		return fmt.Errorf(
			"%w: Community SQLite synchronous mode is %d; expected FULL",
			community.ErrCommunityDataIntegrity,
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
			return fmt.Errorf(
				"%s: %w: %v",
				operation,
				community.ErrCommunityStorageBusy,
				err,
			)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
