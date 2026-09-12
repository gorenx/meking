package sqlite_test

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/memoria-space/meking/epoch"
	"github.com/memoria-space/meking/epoch/adapter/sqlite"
)

func TestConcurrentEpochPublishersAllowOneCurrentChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "epoch.sqlite")
	first, _ := openEpochStore(t, path)
	ctx := testZoneContext(t)
	initial, err := first.Publish(
		ctx,
		epoch.PublicationTarget{
			Knowledge:   knowledgeVersions(1),
			StructureID: structureID,
		},
		corporaID,
		time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("publish initial Epoch: %v", err)
	}
	second, _ := openEpochStore(t, path)
	stores := []*sqlite.Store{first, second}
	start := make(chan struct{})
	results := make(chan error, len(stores))
	var wait sync.WaitGroup
	for index, store := range stores {
		index, store := index, store
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, publishErr := store.Publish(
				ctx,
				epoch.PublicationTarget{
					ExpectedEpoch: initial.ID,
					Knowledge:     knowledgeVersions(2),
					StructureID:   nextStructureID,
				},
				corporaID,
				time.Date(2026, 7, 26, 12, index+1, 0, 0, time.UTC),
			)
			results <- publishErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	succeeded := 0
	conflicted := 0
	for result := range results {
		switch {
		case result == nil:
			succeeded++
		case errors.Is(result, epoch.ErrEpochConflict):
			conflicted++
		default:
			t.Fatalf("concurrent Publish() error = %v", result)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent results: succeeded=%d conflicted=%d", succeeded, conflicted)
	}
	current, err := first.Current(ctx)
	if err != nil || current.ID != 2 || !current.Knowledge.Equal(knowledgeVersions(2)) {
		t.Fatalf("Current() = (%+v, %v)", current, err)
	}
}
