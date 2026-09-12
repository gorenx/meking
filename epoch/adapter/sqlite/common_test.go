package sqlite_test

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"

	epochsqlite "github.com/memoria-space/meking/epoch/adapter/sqlite"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/zone"
	_ "modernc.org/sqlite"
)

const testZoneID = "10000000-0000-4000-8000-000000000001"

func testZoneContext(t *testing.T) context.Context {
	t.Helper()
	return zoneContext(t, testZoneID)
}

func zoneContext(t *testing.T, zoneID string) context.Context {
	t.Helper()
	ctx, err := zone.NewContext(t.Context(), zone.ID(zoneID))
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func knowledgeVersions(version knowledge.Version) knowledge.Manifest {
	return knowledge.Manifest{
		Entities: []knowledge.Reference[knowledge.EntityID]{
			{ID: "44444444-4444-4444-8444-444444444444", Version: version},
		},
	}
}

func openEpochStore(t *testing.T, path string) (*epochsqlite.Store, *sql.DB) {
	t.Helper()
	database, err := openEpochDatabase(path)
	if err != nil {
		t.Fatalf("open Epoch database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close Epoch database: %v", err)
		}
	})
	store, err := epochsqlite.NewStore(database)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, database
}

func openEpochDatabase(path string) (*sql.DB, error) {
	location := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := location.Query()
	query.Set("mode", "rwc")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(OFF)")
	query.Add("_pragma", "synchronous(FULL)")
	location.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(9)
	database.SetMaxIdleConns(9)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}
