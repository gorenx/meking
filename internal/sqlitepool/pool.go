// Package sqlitepool owns the process policy for caller-owned SQLite
// connection pools. Domain adapters still own their schemas and transactions.
package sqlitepool

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const connectionLimit = 9

// Open creates the connection pool that the composition root passes to one
// domain SQLite adapter. It applies process-wide connection settings to every
// physical connection through the DSN; it does not inspect or mutate a domain
// schema.
func Open(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open SQLite database: path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("open SQLite database: path %q must be absolute", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create SQLite database directory: %w", err)
	}

	location := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := location.Query()
	query.Set("mode", "rwc")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(OFF)")
	query.Add("_pragma", "synchronous(FULL)")
	location.RawQuery = query.Encode()

	database, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, fmt.Errorf("open SQLite connection pool: %w", err)
	}
	database.SetMaxOpenConns(connectionLimit)
	database.SetMaxIdleConns(connectionLimit)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("connect SQLite database: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("protect SQLite database file: %w", err)
	}
	return database, nil
}
