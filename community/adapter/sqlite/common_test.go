package sqlite_test

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/memoria-space/meking/community"
	communitysqlite "github.com/memoria-space/meking/community/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
	_ "modernc.org/sqlite"
)

const (
	communityTestZoneID zone.ID = "10000000-0000-4000-8000-000000000001"
	entityA                     = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	entityB                     = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	entityC                     = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	relation                    = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
)

func communityTestContext(t *testing.T) context.Context {
	t.Helper()
	return communityZoneContext(t, communityTestZoneID)
}

func communityZoneContext(t *testing.T, id zone.ID) context.Context {
	t.Helper()
	ctx, err := zone.NewContext(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func openCommunityStore(
	t *testing.T,
	path string,
) (*communitysqlite.Store, *sql.DB) {
	t.Helper()
	database, err := openCommunityDatabase(path)
	if err != nil {
		t.Fatalf("open Community database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close Community database: %v", err)
		}
	})
	store, err := communitysqlite.NewStore(database)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, database
}

func openCommunityDatabase(path string) (*sql.DB, error) {
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

func communitySetInput() (
	community.Hierarchy,
	[]community.EntityReference,
	[]community.RelationReference,
) {
	return community.Hierarchy{Communities: []community.Community{
			{
				ID: 0, Level: 0, ParentID: -1,
				Nodes: []string{entityA, entityB, entityC}, Final: false,
			},
			{
				ID: 1, Level: 1, ParentID: 0,
				Nodes: []string{entityA, entityB}, Final: true,
			},
			{
				ID: 2, Level: 1, ParentID: 0,
				Nodes: []string{entityC}, Final: true,
			},
		}},
		[]community.EntityReference{
			{ID: entityA, Version: 2},
			{ID: entityB, Version: 3},
			{ID: entityC, Version: 4},
		},
		[]community.RelationReference{{ID: relation, Version: 5}}
}

func persistCommunitySet(
	t *testing.T,
	store *communitysqlite.Store,
) community.CommunitySet {
	t.Helper()
	hierarchy, entities, relations := communitySetInput()
	set, err := community.NewCommunitySet(
		hierarchy,
		entities,
		relations,
		community.DefaultDetectConfig(),
	)
	if err != nil {
		t.Fatalf("NewCommunitySet: %v", err)
	}
	if err := store.Save(communityTestContext(t), set); err != nil {
		t.Fatalf("save CommunitySet: %v", err)
	}
	return set
}

func communityCandidateInput() community.Graph {
	_, entities, relations := communitySetInput()
	return community.Graph{
		Entities: entities,
		Relations: []community.GraphRelation{{
			Reference:      relations[0],
			SourceEntityID: entityA,
			TargetEntityID: entityB,
			Weight:         1,
		}},
	}
}
