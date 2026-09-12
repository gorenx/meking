// Package sqlite persists Zone definitions in a caller-owned shared database.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/zone"
)

const schemaVersion = 1

//go:embed schema.sql
var schema string

// Store implements Zone Definition persistence without owning the database.
type Store struct {
	database *sql.DB
}

var _ zone.DefinitionStore = (*Store)(nil)

// NewStore initializes and validates the Zone-owned schema.
func NewStore(database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("create Zone SQLite Store: database is required")
	}
	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("connect Zone SQLite: %w", err)
	}
	if err := initialize(database); err != nil {
		return nil, err
	}
	return &Store{database: database}, nil
}

// Create persists one immutable Zone definition.
func (store *Store) Create(ctx context.Context, definition zone.Definition) error {
	_, err := store.database.ExecContext(
		ctx,
		`INSERT INTO zones (id, parent_zone_id, user_id, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		string(definition.ID),
		optionalID(definition.ParentID),
		optionalString(definition.UserID),
		string(definition.Role),
		definition.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("create Zone Definition %q: %w", definition.ID, err)
	}
	return nil
}

// Resolve restores one exact Zone definition.
func (store *Store) Resolve(ctx context.Context, id zone.ID) (zone.Definition, error) {
	var parentID, userID sql.NullString
	var role, createdAt string
	err := store.database.QueryRowContext(
		ctx,
		`SELECT parent_zone_id, user_id, role, created_at FROM zones WHERE id = ?`,
		string(id),
	).Scan(&parentID, &userID, &role, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return zone.Definition{}, zone.ErrNotFound
	}
	if err != nil {
		return zone.Definition{}, fmt.Errorf("resolve Zone Definition %q: %w", id, err)
	}
	return restoreDefinition(id, parentID, userID, role, createdAt)
}

// ResolveRoot restores the Root Zone assigned to one User.
func (store *Store) ResolveRoot(ctx context.Context, userID string) (zone.Definition, error) {
	var id, role, createdAt string
	var parentID, storedUserID sql.NullString
	err := store.database.QueryRowContext(
		ctx,
		`SELECT id, parent_zone_id, user_id, role, created_at FROM zones WHERE user_id = ?`,
		userID,
	).Scan(&id, &parentID, &storedUserID, &role, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return zone.Definition{}, zone.ErrNotFound
	}
	if err != nil {
		return zone.Definition{}, fmt.Errorf("resolve User Root Zone: %w", err)
	}
	parsedID, err := zone.ParseID(id)
	if err != nil {
		return zone.Definition{}, err
	}
	return restoreDefinition(parsedID, parentID, storedUserID, role, createdAt)
}

// List restores all Zone definitions in deterministic creation order.
func (store *Store) List(ctx context.Context) (_ []zone.Definition, resultErr error) {
	rows, err := store.database.QueryContext(
		ctx,
		`SELECT id, parent_zone_id, user_id, role, created_at FROM zones ORDER BY created_at, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list Zone Definitions: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, rows.Close()) }()

	definitions := make([]zone.Definition, 0)
	for rows.Next() {
		var id, role, createdAt string
		var parentID, userID sql.NullString
		if err := rows.Scan(&id, &parentID, &userID, &role, &createdAt); err != nil {
			return nil, fmt.Errorf("scan Zone Definition: %w", err)
		}
		parsedID, err := zone.ParseID(id)
		if err != nil {
			return nil, fmt.Errorf("restore Zone Definition: %w", err)
		}
		definition, err := restoreDefinition(parsedID, parentID, userID, role, createdAt)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Zone Definitions: %w", err)
	}
	return definitions, nil
}

func initialize(database *sql.DB) (resultErr error) {
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve Zone SQLite initialization connection: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, connection.Close()) }()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin Zone SQLite initialization: %w", err)
	}
	transactionOpen := true
	defer func() {
		if transactionOpen {
			_, rollbackErr := connection.ExecContext(ctx, "ROLLBACK")
			resultErr = errors.Join(resultErr, rollbackErr)
		}
	}()

	var version int
	err = connection.QueryRowContext(
		ctx,
		"SELECT version FROM zone_schema WHERE id = 1",
	).Scan(&version)
	switch {
	case err == nil:
		if version != schemaVersion {
			return fmt.Errorf(
				"open Zone SQLite: unsupported schema version %d; expected %d",
				version,
				schemaVersion,
			)
		}
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("open Zone SQLite: schema version row is missing")
	default:
		versionReadErr := err
		if _, err := connection.ExecContext(ctx, schema); err != nil {
			return errors.Join(
				fmt.Errorf("read Zone SQLite schema version: %w", versionReadErr),
				fmt.Errorf("initialize Zone SQLite schema: %w", err),
			)
		}
	}
	if _, err := connection.ExecContext(
		ctx,
		`SELECT id, parent_zone_id, user_id, role, created_at FROM zones LIMIT 0`,
	); err != nil {
		return fmt.Errorf("validate Zone SQLite schema: %w", err)
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit Zone SQLite initialization: %w", err)
	}
	transactionOpen = false
	return nil
}

func restoreDefinition(
	id zone.ID,
	parentID sql.NullString,
	userID sql.NullString,
	role string,
	createdAtValue string,
) (zone.Definition, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtValue)
	if err != nil {
		return zone.Definition{}, fmt.Errorf(
			"%w: Zone %q has invalid creation time: %v",
			zone.ErrInvalidDefinition,
			id,
			err,
		)
	}
	switch zone.Role(role) {
	case zone.RoleRoot:
		if parentID.Valid {
			return zone.Definition{}, fmt.Errorf(
				"%w: Root Zone %q has a Parent",
				zone.ErrInvalidDefinition,
				id,
			)
		}
		return zone.NewRootDefinition(id, userID.String, createdAt)
	case zone.RoleChild:
		if !parentID.Valid {
			return zone.Definition{}, zone.ErrParentRequired
		}
		if userID.Valid {
			return zone.Definition{}, fmt.Errorf(
				"%w: Child Zone %q identifies User %q",
				zone.ErrInvalidDefinition,
				id,
				userID.String,
			)
		}
		parsedParent, err := zone.ParseID(parentID.String)
		if err != nil {
			return zone.Definition{}, err
		}
		return zone.NewChildDefinition(id, parsedParent, createdAt)
	default:
		return zone.Definition{}, fmt.Errorf(
			"%w: Zone %q has unsupported role %q",
			zone.ErrInvalidDefinition,
			id,
			role,
		)
	}
}

func optionalID(id *zone.ID) any {
	if id == nil {
		return nil
	}
	return string(*id)
}

func optionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
