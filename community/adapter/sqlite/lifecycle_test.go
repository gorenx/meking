package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/memoria-space/meking/community"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

func TestCommunitySetRoundTripsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.sqlite")
	store, database := openCommunityStore(t, path)
	persisted := persistCommunitySet(t, store)
	id := persisted.ID
	if err := community.ValidateCommunitySetID(id); err != nil {
		t.Fatalf("CommunitySet ID: %v", err)
	}
	before, err := store.Load(communityTestContext(t), id)
	if err != nil {
		t.Fatalf("CommunitySet before reopen: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close Community database before reopen: %v", err)
	}

	reopened, _ := openCommunityStore(t, path)
	after, err := reopened.Load(communityTestContext(t), id)
	if err != nil {
		t.Fatalf("CommunitySet after reopen: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("CommunitySet after reopen\n got: %#v\nwant: %#v", after, before)
	}
	if len(after.Communities) != 3 ||
		after.Communities[1].ParentID == nil ||
		*after.Communities[1].ParentID != after.Communities[0].ID {
		t.Fatalf("persisted hierarchy = %#v", after.Communities)
	}
}

func TestCommunityCandidateRecordsDetectionContract(t *testing.T) {
	store, _ := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	graph := communityCandidateInput()
	config := community.DetectConfig{
		MaxClusterSize:               2,
		UseLargestConnectedComponent: false,
		Seed:                         42,
	}
	created, err := community.Detect(
		communityTestContext(t),
		graph,
		config,
	)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if err := store.Save(communityTestContext(t), created); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if created.DetectorVersion != community.CurrentDetectorVersion {
		t.Fatalf(
			"DetectorVersion = %d, want %d",
			created.DetectorVersion,
			community.CurrentDetectorVersion,
		)
	}
	if !reflect.DeepEqual(created.DetectionConfig, config) {
		t.Fatalf("DetectionConfig = %#v, want %#v", created.DetectionConfig, config)
	}
	stored, err := store.Load(communityTestContext(t), created.ID)
	if err != nil {
		t.Fatalf("CommunitySet: %v", err)
	}
	if !reflect.DeepEqual(stored, created) {
		t.Fatalf("stored CommunitySet\n got: %#v\nwant: %#v", stored, created)
	}
}

func TestCommunityStoreRejectsReusedSetID(t *testing.T) {
	store, _ := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	persisted := persistCommunitySet(t, store)
	id := persisted.ID
	set, err := store.Load(communityTestContext(t), id)
	if err != nil {
		t.Fatalf("CommunitySet: %v", err)
	}
	if err := store.Save(communityTestContext(t), set); !errors.Is(err, community.ErrCommunitySetConflict) {
		t.Fatalf("Store.Save duplicate error = %v", err)
	}
}

func TestCommunitySetUnknownID(t *testing.T) {
	store, _ := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	_, err := store.Load(
		communityTestContext(t),
		"11111111-1111-4111-8111-111111111111",
	)
	if !errors.Is(err, community.ErrCommunitySetNotFound) {
		t.Fatalf("CommunitySet unknown error = %v", err)
	}
}

func TestCommunityStoreJoinsProjectTransaction(t *testing.T) {
	store, database := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "project.sqlite"),
	)
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
	scope, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatalf("create transaction scope: %v", err)
	}
	rollback := errors.New("rollback Community write")
	err = scope.WithTx(communityTestContext(t), func(ctx context.Context) error {
		if err := store.Save(ctx, set); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("WithTx error = %v, want rollback marker", err)
	}
	if _, err := store.Load(communityTestContext(t), set.ID); !errors.Is(err, community.ErrCommunitySetNotFound) {
		t.Fatalf("rolled-back CommunitySet read error = %v", err)
	}
}
