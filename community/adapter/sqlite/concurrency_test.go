package sqlite_test

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/memoria-space/meking/community"
	communitysqlite "github.com/memoria-space/meking/community/adapter/sqlite"
)

func TestConcurrentStoresInitializeOneCommunitySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.sqlite")
	start := make(chan struct{})
	failures := make(chan error, 8)
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			database, err := openCommunityDatabase(path)
			if err == nil {
				_, err = communitysqlite.NewStore(database)
			}
			if database != nil {
				err = errors.Join(err, database.Close())
			}
			failures <- err
		}()
	}
	close(start)
	wait.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Errorf("concurrent NewStore: %v", err)
		}
	}
}

func TestConcurrentCommunityCandidatesRemainIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.sqlite")
	first, _ := openCommunityStore(t, path)
	second, _ := openCommunityStore(t, path)
	graph := communityCandidateInput()
	concurrentContext := communityTestContext(t)

	start := make(chan struct{})
	results := make(chan communityCandidateResult, 2)
	var wait sync.WaitGroup
	for _, store := range []*communitysqlite.Store{first, second} {
		store := store
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			set, err := community.Detect(
				concurrentContext,
				graph,
				community.DefaultDetectConfig(),
			)
			if err == nil {
				err = store.Save(concurrentContext, set)
			}
			results <- communityCandidateResult{id: set.ID, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	ids := make(map[community.CommunitySetID]struct{}, 2)
	for result := range results {
		if result.err != nil {
			t.Fatalf("Detect or Save: %v", result.err)
		}
		ids[result.id] = struct{}{}
	}
	if len(ids) != 2 {
		t.Fatalf("concurrent CommunitySet IDs = %#v", ids)
	}
}

type communityCandidateResult struct {
	id  community.CommunitySetID
	err error
}
