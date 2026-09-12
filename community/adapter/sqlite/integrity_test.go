package sqlite_test

import (
	"database/sql"
	"errors"
	communitysqlite "github.com/memoria-space/meking/community/adapter/sqlite"
	"path/filepath"
	"strings"
	"testing"

	"github.com/memoria-space/meking/community"
)

func TestCommunityReaderRejectsMemberWithoutEntityVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.sqlite")
	store, _ := openCommunityStore(t, path)
	persisted := persistCommunitySet(t, store)
	id := persisted.ID

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw Community database: %v", err)
	}
	if _, err := raw.Exec(
		`DELETE FROM community_entities
		 WHERE zone_id = ? AND community_set_id = ? AND entity_id = ?`,
		communityTestZoneID,
		id,
		entityA,
	); err != nil {
		t.Fatalf("delete Entity reference: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw Community database: %v", err)
	}

	if _, err := store.Load(
		communityTestContext(t),
		id,
	); !errors.Is(err, community.ErrCommunityDataIntegrity) {
		t.Fatalf("CommunitySet corrupt reference error = %v", err)
	}
}

func TestNewCommunityStoreRequiresConfiguredCallerOwnedDatabase(t *testing.T) {
	if _, err := communitysqlite.NewStore(nil); err == nil {
		t.Fatal("NewStore(nil) succeeded")
	}
	database, err := sql.Open(
		"sqlite",
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	if err != nil {
		t.Fatalf("open unconfigured database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := communitysqlite.NewStore(
		database,
	); !errors.Is(err, community.ErrCommunityDataIntegrity) {
		t.Fatalf(
			"NewStore unconfigured error = %v, want ErrCommunityDataIntegrity",
			err,
		)
	}
}

func TestCommunityStoreRejectsNoncurrentSchemaWithoutMigration(t *testing.T) {
	database, err := openCommunityDatabase(
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	if err != nil {
		t.Fatalf("open Community database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := communitysqlite.NewStore(database); err != nil {
		t.Fatalf("initialize Community Store: %v", err)
	}
	if _, err := database.Exec("UPDATE community_schema SET version = 2 WHERE id = 1"); err != nil {
		t.Fatalf("set old Community schema version: %v", err)
	}
	if _, err := communitysqlite.NewStore(database); err == nil ||
		!strings.Contains(err.Error(), "unsupported schema version 2") {
		t.Fatalf("NewStore old schema error = %v", err)
	}
}

func TestCommunityStoreInitializesAlongsideUnrelatedProjectObjects(t *testing.T) {
	database, err := openCommunityDatabase(
		filepath.Join(t.TempDir(), "project.sqlite"),
	)
	if err != nil {
		t.Fatalf("open Project database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`CREATE TABLE project_marker (id INTEGER PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("create unrelated Project object: %v", err)
	}
	if _, err := database.Exec(`
		CREATE TRIGGER project_marker_insert
		AFTER INSERT ON project_marker BEGIN SELECT NEW.id; END
	`); err != nil {
		t.Fatalf("create unrelated Project trigger: %v", err)
	}
	if _, err := communitysqlite.NewStore(database); err != nil {
		t.Fatalf("NewStore with unrelated objects: %v", err)
	}
}

func TestCommunitySchemaHasNoForeignKeysOrTriggers(t *testing.T) {
	_, database := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	var foreignKeys, triggers int
	if err := database.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if err := database.QueryRow(
		`SELECT count(*) FROM sqlite_schema WHERE type = 'trigger'`,
	).Scan(&triggers); err != nil {
		t.Fatalf("count triggers: %v", err)
	}
	if foreignKeys != 0 || triggers != 0 {
		t.Fatalf("Community schema foreign_keys=%d triggers=%d", foreignKeys, triggers)
	}
}
