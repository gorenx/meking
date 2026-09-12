// Package sqlite persists Knowledge business facts in the Project SQLite
// database while keeping every operation isolated by Zone.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	domain "github.com/memoria-space/meking/knowledge"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
	sqlitedriver "modernc.org/sqlite"
)

const (
	walEnableRetryWindow   = 5 * time.Second
	walEnableRetryInterval = 10 * time.Millisecond
)

type Database struct {
	database *sql.DB
}

func New(database *sql.DB) (*Database, error) {
	if database == nil {
		return nil, errors.New("create Knowledge Database: SQLite database is required")
	}
	if err := database.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("connect Knowledge Database: %w", err)
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
	result := &Database{database: database}
	if err := result.CheckIntegrity(context.Background()); err != nil {
		return nil, fmt.Errorf("open Knowledge Database: %w", err)
	}
	return result, nil
}

func initialize(database *sql.DB) (resultErr error) {
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		return classifySQLite("reserve Knowledge schema connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Knowledge schema transaction", err)
	}
	open := true
	defer func() {
		if open {
			_, rollbackErr := connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
			resultErr = errors.Join(resultErr, rollbackErr)
		}
	}()

	var tables int
	if err := connection.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'knowledge_schema'",
	).Scan(&tables); err != nil {
		return classifySQLite("inspect Knowledge schema", err)
	}
	if tables == 0 {
		if _, err := connection.ExecContext(ctx, schema); err != nil {
			return classifySQLite("initialize Knowledge schema", err)
		}
	} else {
		var version int
		if err := connection.QueryRowContext(ctx,
			"SELECT version FROM knowledge_schema WHERE id = 1",
		).Scan(&version); err != nil {
			return fmt.Errorf("%w: read Knowledge schema version: %v", domain.ErrDataIntegrity, err)
		}
		if version != schemaVersion {
			return fmt.Errorf("open Knowledge Database: unsupported schema version %d; expected %d", version, schemaVersion)
		}
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Knowledge schema transaction", err)
	}
	open = false
	return nil
}

func (knowledge *Database) CheckIntegrity(ctx context.Context) error {
	if knowledge == nil || knowledge.database == nil {
		return errors.New("check Knowledge Database integrity: Database is not configured")
	}
	checks := []struct {
		name  string
		query string
	}{
		{"Entity Version without identity", `SELECT v.knowledge_id FROM knowledge_versions v LEFT JOIN entity_identities i ON i.zone_id=v.zone_id AND i.entity_id=v.knowledge_id WHERE v.knowledge_kind='entity' AND i.entity_id IS NULL LIMIT 1`},
		{"Relation Version without identity", `SELECT v.knowledge_id FROM knowledge_versions v LEFT JOIN relation_identities i ON i.zone_id=v.zone_id AND i.relation_id=v.knowledge_id WHERE v.knowledge_kind='relation' AND i.relation_id IS NULL LIMIT 1`},
		{"Claim Version without identity", `SELECT v.knowledge_id FROM knowledge_versions v LEFT JOIN claim_identities i ON i.zone_id=v.zone_id AND i.claim_id=v.knowledge_id WHERE v.knowledge_kind='claim' AND i.claim_id IS NULL LIMIT 1`},
	}
	for _, check := range checks {
		var id string
		err := knowledge.database.QueryRowContext(ctx, check.query).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return classifySQLite("check "+check.name, err)
		}
		return fmt.Errorf("%w: %s %q", domain.ErrDataIntegrity, check.name, id)
	}
	return nil
}

func (knowledge *Database) reader(ctx context.Context) (statements, error) {
	if knowledge == nil || knowledge.database == nil {
		return statements{}, errors.New("read Knowledge Database: Database is not configured")
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return statements{}, err
	}
	if executor, txErr := transactionsqlite.Current(ctx, knowledge.database); txErr == nil {
		return newStatements(executor, string(zoneID)), nil
	} else if !errors.Is(txErr, transactionsqlite.ErrNoTransaction) {
		return statements{}, txErr
	}
	return newStatements(knowledge.database, string(zoneID)), nil
}

func (knowledge *Database) writer(ctx context.Context) (statements, error) {
	if knowledge == nil || knowledge.database == nil {
		return statements{}, errors.New("write Knowledge Database: Database is not configured")
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return statements{}, err
	}
	executor, err := transactionsqlite.Current(ctx, knowledge.database)
	if err != nil {
		return statements{}, err
	}
	return newStatements(executor, string(zoneID)), nil
}

func enableWAL(database *sql.DB) error {
	deadline := time.Now().Add(walEnableRetryWindow)
	for {
		var mode string
		err := database.QueryRow("PRAGMA journal_mode = WAL").Scan(&mode)
		if err == nil {
			if mode != "wal" {
				return fmt.Errorf("%w: Knowledge SQLite journal mode is %q; expected wal", domain.ErrDataIntegrity, mode)
			}
			return nil
		}
		classified := classifySQLite("enable Knowledge SQLite WAL", err)
		if !errors.Is(classified, domain.ErrStorageBusy) || !time.Now().Before(deadline) {
			return classified
		}
		time.Sleep(walEnableRetryInterval)
	}
}

func verifyConnection(ctx context.Context, database *sql.DB) error {
	for pragma, expected := range map[string]int{"foreign_keys": 0, "busy_timeout": 5000, "synchronous": 2} {
		var actual int
		if err := database.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&actual); err != nil {
			return fmt.Errorf("verify Knowledge SQLite %s: %w", pragma, err)
		}
		if actual != expected {
			return fmt.Errorf("%w: Knowledge SQLite %s is %d; expected %d", domain.ErrDataIntegrity, pragma, actual, expected)
		}
	}
	return nil
}

func classifySQLite(operation string, err error) error {
	if err == nil {
		return nil
	}
	var sqliteError *sqlitedriver.Error
	if errors.As(err, &sqliteError) && (sqliteError.Code()&0xff == 5 || sqliteError.Code()&0xff == 6) {
		return fmt.Errorf("%s: %w: %v", operation, domain.ErrStorageBusy, err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func constraint(err error) bool {
	if err == nil {
		return false
	}
	var sqliteError *sqlitedriver.Error
	return errors.As(err, &sqliteError) && sqliteError.Code()&0xff == 19
}

func storedHash(value []byte) (domain.Hash, error) {
	if len(value) != 32 {
		return domain.Hash{}, fmt.Errorf("%w: stored Content Hash has %d bytes", domain.ErrDataIntegrity, len(value))
	}
	var hash domain.Hash
	copy(hash[:], value)
	return hash, nil
}

func boolValue(value int64) (bool, error) {
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("%w: stored boolean is %d", domain.ErrDataIntegrity, value)
	}
}
