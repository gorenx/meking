package sqlite_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/memoria-space/meking/epoch"
)

const (
	corporaID       = epoch.CorporaID("11111111-1111-4111-8111-111111111111")
	structureID     = epoch.StructureID("22222222-2222-4222-8222-222222222222")
	nextStructureID = epoch.StructureID("33333333-3333-4333-8333-333333333333")
)

func TestEpochStorePublishesAndReadsExactHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "epoch.sqlite")
	store, _ := openEpochStore(t, path)
	ctx := testZoneContext(t)
	if _, err := store.Current(ctx); !errors.Is(err, epoch.ErrEpochNotFound) {
		t.Fatalf("Current() before publication error = %v", err)
	}
	firstTime := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	first, err := store.Publish(
		ctx,
		epoch.PublicationTarget{
			Knowledge:   knowledgeVersions(4),
			StructureID: structureID,
		},
		corporaID,
		firstTime,
	)
	if err != nil {
		t.Fatalf("publish initial Epoch: %v", err)
	}
	if first.ID != 1 || !first.Knowledge.Equal(knowledgeVersions(4)) {
		t.Fatalf("initial Epoch = %+v", first)
	}
	secondTime := firstTime.Add(time.Minute)
	second, err := store.Publish(
		ctx,
		epoch.PublicationTarget{
			ExpectedEpoch: first.ID,
			Knowledge:     knowledgeVersions(4),
			StructureID:   nextStructureID,
		},
		corporaID,
		secondTime,
	)
	if err != nil {
		t.Fatalf("publish same-change Epoch: %v", err)
	}
	if second.ID != 2 || !second.Knowledge.Equal(first.Knowledge) {
		t.Fatalf("second Epoch = %+v", second)
	}
	current, err := store.Current(ctx)
	if err != nil || !current.Equal(second) {
		t.Fatalf("Current() = (%+v, %v), want %+v", current, err, second)
	}
	historical, err := store.Load(ctx, first.ID)
	if err != nil || !historical.Equal(first) {
		t.Fatalf("Load(first) = (%+v, %v), want %+v", historical, err, first)
	}

	reopened, _ := openEpochStore(t, path)
	current, err = reopened.Current(ctx)
	if err != nil || !current.Equal(second) {
		t.Fatalf("reopened Current() = (%+v, %v), want %+v", current, err, second)
	}
}

func TestEpochStoreRejectsConflictAndInvalidKnowledgeWithoutChangingCurrent(t *testing.T) {
	store, _ := openEpochStore(t, filepath.Join(t.TempDir(), "epoch.sqlite"))
	ctx := testZoneContext(t)
	publishedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	first, err := store.Publish(
		ctx,
		epoch.PublicationTarget{
			Knowledge:   knowledgeVersions(8),
			StructureID: structureID,
		},
		corporaID,
		publishedAt,
	)
	if err != nil {
		t.Fatalf("publish initial Epoch: %v", err)
	}
	_, err = store.Publish(
		ctx,
		epoch.PublicationTarget{
			Knowledge:   knowledgeVersions(9),
			StructureID: nextStructureID,
		},
		corporaID,
		publishedAt.Add(time.Minute),
	)
	if !errors.Is(err, epoch.ErrEpochConflict) {
		t.Fatalf("stale publication error = %v", err)
	}
	_, err = store.Publish(
		ctx,
		epoch.PublicationTarget{
			ExpectedEpoch: first.ID,
			Knowledge:     knowledgeVersions(0),
			StructureID:   nextStructureID,
		},
		corporaID,
		publishedAt.Add(time.Minute),
	)
	if !errors.Is(err, epoch.ErrInvalidEpoch) {
		t.Fatalf("regressing publication error = %v", err)
	}
	current, currentErr := store.Current(ctx)
	if currentErr != nil || !current.Equal(first) {
		t.Fatalf("Current() after rejection = (%+v, %v), want %+v", current, currentErr, first)
	}
}
