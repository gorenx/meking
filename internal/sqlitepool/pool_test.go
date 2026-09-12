package sqlitepool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenConfiguresEveryConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "knowledge.sqlite")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	for range connectionLimit {
		connection, err := database.Conn(t.Context())
		if err != nil {
			t.Fatalf("Conn() error = %v", err)
		}
		defer func() {
			if err := connection.Close(); err != nil {
				t.Errorf("connection Close() error = %v", err)
			}
		}()
		var foreignKeys, busyTimeout, synchronous int
		if err := connection.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			t.Fatalf("read foreign_keys: %v", err)
		}
		if err := connection.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatalf("read busy_timeout: %v", err)
		}
		if err := connection.QueryRowContext(t.Context(), "PRAGMA synchronous").Scan(&synchronous); err != nil {
			t.Fatalf("read synchronous: %v", err)
		}
		if foreignKeys != 0 || busyTimeout != 5000 || synchronous != 2 {
			t.Fatalf(
				"SQLite pragmas = foreign_keys %d busy_timeout %d synchronous %d",
				foreignKeys,
				busyTimeout,
				synchronous,
			)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("SQLite database permissions = %o, want 600", got)
	}
}

func TestOpenRejectsRelativePath(t *testing.T) {
	if _, err := Open("knowledge.sqlite"); err == nil {
		t.Fatal("Open() accepted relative path")
	}
}
