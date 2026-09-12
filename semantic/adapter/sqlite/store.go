// Package sqlite implements Semantic Namespace persistence in a caller-owned
// SQLite database. Namespace membership is partitioned by the Zone bound to
// each operation Context; canonical vectors remain reusable by opaque ID.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/memoria-space/meking/semantic"
	_ "modernc.org/sqlite/vec"
)

const (
	walEnableRetryWindow     = 5 * time.Second
	walEnableRetryInterval   = 10 * time.Millisecond
	expectedSQLiteVecVersion = "v0.1.9"
	vectorLookupBatchSize    = 500
)

type Store struct {
	database *sql.DB
	logger   *slog.Logger
}

var _ semantic.NamespaceStore = (*Store)(nil)

func NewStore(database *sql.DB, model string, logger *slog.Logger) (*Store, error) {
	if database == nil {
		return nil, errors.New("create Semantic SQLite Store: database is required")
	}
	if logger == nil {
		return nil, errors.New("create Semantic SQLite Store: logger is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("create Semantic SQLite Store: embedding model is required")
	}
	if err := database.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("connect Semantic SQLite: %w", err)
	}
	if err := verifyConnection(context.Background(), database); err != nil {
		return nil, err
	}
	if err := enableWAL(database); err != nil {
		return nil, err
	}
	if err := initialize(database); err != nil {
		return nil, err
	}
	store := &Store{database: database, logger: logger}
	if err := store.configure(model); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) configure(model string) error {
	ctx := context.Background()
	var dimension int64
	err := s.write(ctx, func(statements statements, _ sqlExecutor) error {
		configured, err := statements.GetVectorDatabase(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			if err := statements.CreateVectorDatabase(ctx, model); err != nil {
				return classifySQLite("configure Semantic embedding model", err)
			}
			return nil
		}
		if err != nil {
			return classifySQLite("read Semantic embedding model", err)
		}
		if configured.Model != model {
			return fmt.Errorf(
				"Semantic SQLite embedding model is %q; configured %q", configured.Model, model,
			)
		}
		if configured.Dimension < 0 || int64(int(configured.Dimension)) != configured.Dimension {
			return errors.New("Semantic SQLite embedding dimension is invalid")
		}
		dimension = configured.Dimension
		return nil
	})
	if err != nil || dimension == 0 {
		return err
	}
	_, err = existingVectorTable(ctx, s.database, int(dimension))
	return err
}

func (s *Store) Lookup(ctx context.Context, ids []string) ([]semantic.Vector, error) {
	prepared, err := semantic.PrepareIDs(ids)
	if err != nil {
		return nil, err
	}
	if len(prepared) == 0 {
		return []semantic.Vector{}, nil
	}
	byID := make(map[string]semantic.Vector, len(prepared))
	statements := newStatements(s.database)
	for start := 0; start < len(prepared); start += vectorLookupBatchSize {
		end := min(start+vectorLookupBatchSize, len(prepared))
		rows, err := statements.ListVectors(ctx, prepared[start:end])
		if err != nil {
			return nil, classifySQLite("query Semantic vector cache", err)
		}
		for _, row := range rows {
			var values []float64
			if err := json.Unmarshal([]byte(row.Embedding), &values); err != nil {
				return nil, fmt.Errorf("decode cached Semantic vector %q: %w", row.VectorID, err)
			}
			if err := semantic.ValidateVector(values); err != nil {
				return nil, fmt.Errorf("validate cached Semantic vector %q: %w", row.VectorID, err)
			}
			byID[row.VectorID] = semantic.Vector{ID: row.VectorID, Values: values}
		}
	}
	result := make([]semantic.Vector, 0, len(byID))
	for _, id := range prepared {
		if vector, exists := byID[id]; exists {
			result = append(result, vector)
		}
	}
	return result, nil
}

func enableWAL(database *sql.DB) error {
	deadline := time.Now().Add(walEnableRetryWindow)
	for {
		var mode string
		err := database.QueryRow("PRAGMA journal_mode = WAL").Scan(&mode)
		if err == nil {
			if mode != "wal" {
				return fmt.Errorf("Semantic SQLite journal mode is %q; expected wal", mode)
			}
			return nil
		}
		if !isBusy(err) || !time.Now().Before(deadline) {
			return classifySQLite("enable Semantic SQLite WAL", err)
		}
		time.Sleep(walEnableRetryInterval)
	}
}

func initialize(database *sql.DB) (resultErr error) {
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		return classifySQLite("reserve Semantic SQLite initialization connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, classifySQLite(
				"release Semantic SQLite initialization connection", closeErr,
			))
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Semantic SQLite initialization", err)
	}
	open := true
	defer func() {
		if open {
			_, rollbackErr := connection.ExecContext(ctx, "ROLLBACK")
			resultErr = errors.Join(resultErr, classifySQLite(
				"roll back Semantic SQLite initialization", rollbackErr,
			))
		}
	}()
	statements := newStatements(connection)
	version, err := statements.GetSchemaVersion(ctx)
	switch {
	case err == nil:
		if version != schemaVersion {
			return fmt.Errorf(
				"open Semantic SQLite: unsupported schema version %d; expected %d",
				version,
				schemaVersion,
			)
		}
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("Semantic SQLite schema version row is missing")
	default:
		versionReadErr := err
		if _, err := connection.ExecContext(ctx, schema); err != nil {
			return errors.Join(
				fmt.Errorf("read Semantic SQLite schema version: %w", versionReadErr),
				classifySQLite("initialize Semantic SQLite schema", err),
			)
		}
		statements = newStatements(connection)
	}
	if _, err := statements.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("validate Semantic SQLite schema: %w", err)
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Semantic SQLite initialization", err)
	}
	open = false
	return nil
}
