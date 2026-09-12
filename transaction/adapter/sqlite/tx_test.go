package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestWithTxCommitsAndRollsBack(t *testing.T) {
	database := openTestDatabase(t, "transaction")
	if _, err := database.Exec(`CREATE TABLE values_table (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	scope, err := New(database)
	if err != nil {
		t.Fatal(err)
	}
	if err := scope.WithTx(t.Context(), func(ctx context.Context) error {
		executor, err := Current(ctx, database)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(ctx, `INSERT INTO values_table (value) VALUES ('committed')`)
		return err
	}); err != nil {
		t.Fatalf("committed WithTx() error = %v", err)
	}
	failure := errors.New("fail transaction")
	if err := scope.WithTx(t.Context(), func(ctx context.Context) error {
		executor, err := Current(ctx, database)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO values_table (value) VALUES ('rolled-back')`); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("rolled-back WithTx() error = %v, want failure", err)
	}
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM values_table`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stored row count = %d, want 1", count)
	}
}

func TestWithTxJoinsNestedWorkAndRejectsForeignScope(t *testing.T) {
	first := openTestDatabase(t, "first")
	second := openTestDatabase(t, "second")
	scope, err := New(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`CREATE TABLE nested_values (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("roll back joined work")
	if err := scope.WithTx(t.Context(), func(ctx context.Context) error {
		if _, err := Current(ctx, second); !errors.Is(err, ErrDifferentDatabase) {
			t.Fatalf("Current(foreign database) error = %v", err)
		}
		if err := scope.WithTx(ctx, func(nested context.Context) error {
			executor, err := Current(nested, first)
			if err != nil {
				return err
			}
			_, err = executor.ExecContext(nested, `INSERT INTO nested_values (value) VALUES ('joined')`)
			return err
		}); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("joined WithTx() error = %v, want outer failure", err)
	}
	var count int
	if err := first.QueryRow(`SELECT count(*) FROM nested_values`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("joined transaction row count = %d, error = %v", count, err)
	}
	if _, err := Current(t.Context(), first); !errors.Is(err, ErrNoTransaction) {
		t.Fatalf("Current(outside callback) error = %v, want ErrNoTransaction", err)
	}
}

func TestWithTxReservesWriterBeforeApplicationReads(t *testing.T) {
	database := openConcurrentTestDatabase(t)
	if _, err := database.Exec(`CREATE TABLE counter (id INTEGER PRIMARY KEY, value INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO counter (id, value) VALUES (1, 0)`); err != nil {
		t.Fatal(err)
	}
	first, err := New(database)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- first.WithTx(t.Context(), func(ctx context.Context) error {
			executor, err := Current(ctx, database)
			if err != nil {
				return err
			}
			var value int
			if err := executor.QueryRowContext(ctx, `SELECT value FROM counter WHERE id = 1`).Scan(&value); err != nil {
				return err
			}
			close(firstEntered)
			<-releaseFirst
			_, err = executor.ExecContext(ctx, `UPDATE counter SET value = ? WHERE id = 1`, value+1)
			return err
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondResult := make(chan error, 1)
	go func() {
		secondResult <- second.WithTx(t.Context(), func(ctx context.Context) error {
			close(secondEntered)
			executor, err := Current(ctx, database)
			if err != nil {
				return err
			}
			var value int
			if err := executor.QueryRowContext(ctx, `SELECT value FROM counter WHERE id = 1`).Scan(&value); err != nil {
				return err
			}
			_, err = executor.ExecContext(ctx, `UPDATE counter SET value = ? WHERE id = 1`, value+1)
			return err
		})
	}()
	premature := false
	select {
	case <-secondEntered:
		premature = true
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstResult; err != nil {
		t.Fatalf("first WithTx() error = %v", err)
	}
	if err := <-secondResult; err != nil {
		t.Fatalf("second WithTx() error = %v", err)
	}
	if premature {
		t.Fatal("second write transaction entered application work before the first committed")
	}
	var value int
	if err := database.QueryRow(`SELECT value FROM counter WHERE id = 1`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != 2 {
		t.Fatalf("counter value = %d, want 2", value)
	}
}

func openTestDatabase(t *testing.T, name string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if err := database.Ping(); err != nil {
		t.Fatal(err)
	}
	return database
}

func openConcurrentTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transactions.sqlite")
	database, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(2000)")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(2)
	database.SetMaxIdleConns(2)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close concurrent database: %v", err)
		}
	})
	if _, err := database.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		t.Fatal(err)
	}
	return database
}
