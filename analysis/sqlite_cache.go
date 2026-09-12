package analysis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/memoria-space/meking/analysis/internal/sqlitecache"
	_ "modernc.org/sqlite"
)

// SQLiteCache stores complete analysis results in a private Project-local
// database whose schema and lifecycle are independent of model response cache.
type SQLiteCache struct {
	db *sql.DB
}

var _ Cache = (*SQLiteCache)(nil)

// OpenSQLiteCache opens or creates an analysis-only cache file.
func OpenSQLiteCache(path string) (*SQLiteCache, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("analysis cache path is required")
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create analysis cache directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open analysis cache: %w", err)
	}
	db.SetMaxOpenConns(1)
	cache := &SQLiteCache{db: db}
	if err := cache.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("protect analysis cache file: %w", err)
	}
	return cache, nil
}

func (c *SQLiteCache) initialize() (resultErr error) {
	ctx := context.Background()
	connection, err := c.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve analysis cache initialization connection: %w", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("release analysis cache initialization connection: %w", closeErr),
			)
		}
	}()

	var journalMode string
	if err := connection.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		return fmt.Errorf("enable analysis cache WAL: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("analysis cache journal mode is %q; expected wal", journalMode)
	}
	if _, err := connection.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("configure analysis cache busy timeout: %w", err)
	}

	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin analysis cache initialization: %w", err)
	}
	transactionOpen := true
	defer func() {
		if !transactionOpen {
			return
		}
		if _, rollbackErr := connection.ExecContext(ctx, "ROLLBACK"); rollbackErr != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("roll back analysis cache initialization: %w", rollbackErr),
			)
		}
	}()

	queries := sqlitecache.New(connection)
	version, err := queries.GetAnalysisCacheSchemaVersion(ctx)
	switch {
	case err == nil:
		if version != sqliteCacheSchemaVersion {
			return fmt.Errorf(
				"open analysis cache: unsupported schema version %d; expected %d",
				version,
				sqliteCacheSchemaVersion,
			)
		}
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("analysis cache schema version row is missing")
	default:
		versionReadErr := err
		if _, err := connection.ExecContext(ctx, sqliteCacheSchema); err != nil {
			return errors.Join(
				fmt.Errorf("read analysis cache schema version: %w", versionReadErr),
				fmt.Errorf("initialize analysis cache schema: %w", err),
			)
		}
	}
	if _, err := queries.ValidateAnalysisCacheSchema(ctx); err != nil {
		return fmt.Errorf("validate analysis cache schema: %w", err)
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit analysis cache initialization: %w", err)
	}
	transactionOpen = false
	return nil
}

// Get returns a defensive copy of one cached complete response.
func (c *SQLiteCache) Get(ctx context.Context, key string) (CacheEntry, bool, error) {
	if c == nil || c.db == nil {
		return CacheEntry{}, false, errors.New("analysis cache is closed")
	}
	if strings.TrimSpace(key) == "" {
		return CacheEntry{}, false, errors.New("analysis cache key is required")
	}
	stored, err := sqlitecache.New(c.db).GetAnalysisCacheEntry(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return CacheEntry{}, false, nil
	}
	if err != nil {
		return CacheEntry{}, false, fmt.Errorf("query analysis cache: %w", err)
	}
	entry := CacheEntry{
		Capability: Capability(stored.Capability),
		Payload:    append([]byte(nil), stored.Payload...),
	}
	if !validCapability(entry.Capability) || len(entry.Payload) == 0 {
		return CacheEntry{}, false, errors.New("analysis cache entry is invalid")
	}
	entry.CreatedAt, err = time.Parse(time.RFC3339Nano, stored.CreatedAt)
	if err != nil {
		return CacheEntry{}, false, fmt.Errorf("parse analysis cache creation time: %w", err)
	}
	return entry, true, nil
}

// Put atomically inserts or replaces one complete response.
func (c *SQLiteCache) Put(ctx context.Context, key string, entry CacheEntry) error {
	if c == nil || c.db == nil {
		return errors.New("analysis cache is closed")
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("analysis cache key is required")
	}
	if !validCapability(entry.Capability) {
		return errors.New("analysis cache capability is invalid")
	}
	if len(entry.Payload) == 0 {
		return errors.New("analysis cache payload is required")
	}
	if entry.CreatedAt.IsZero() {
		return errors.New("analysis cache creation time is required")
	}
	err := sqlitecache.New(c.db).PutAnalysisCacheEntry(ctx, sqlitecache.PutAnalysisCacheEntryParams{
		CacheKey: key, Capability: string(entry.Capability), Payload: entry.Payload,
		CreatedAt: entry.CreatedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("upsert analysis cache: %w", err)
	}
	return nil
}

// Close releases the SQLite connection.
func (c *SQLiteCache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}
