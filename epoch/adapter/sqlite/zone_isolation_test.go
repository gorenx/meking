package sqlite_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/memoria-space/meking/epoch"
	"github.com/memoria-space/meking/zone"
)

func TestEpochStoreIsolatesZones(t *testing.T) {
	store, _ := openEpochStore(t, filepath.Join(t.TempDir(), "epoch.sqlite"))
	zoneA := zoneContext(t, "10000000-0000-4000-8000-000000000001")
	zoneB := zoneContext(t, "20000000-0000-4000-8000-000000000002")
	publishedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	epochA, err := store.Publish(
		zoneA,
		epoch.PublicationTarget{
			Knowledge:   knowledgeVersions(4),
			StructureID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		},
		epoch.CorporaID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		publishedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	epochB, err := store.Publish(
		zoneB,
		epoch.PublicationTarget{
			Knowledge:   knowledgeVersions(9),
			StructureID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		},
		epoch.CorporaID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"),
		publishedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if epochA.ID != 1 || epochB.ID != 1 {
		t.Fatalf("initial Zone Epoch IDs = (%d, %d), want (1, 1)", epochA.ID, epochB.ID)
	}

	currentA, err := store.Current(zoneA)
	if err != nil || !currentA.Equal(epochA) {
		t.Fatalf("Zone A Current() = (%+v, %v), want %+v", currentA, err, epochA)
	}
	currentB, err := store.Current(zoneB)
	if err != nil || !currentB.Equal(epochB) {
		t.Fatalf("Zone B Current() = (%+v, %v), want %+v", currentB, err, epochB)
	}
	loadedA, err := store.Load(zoneA, epochA.ID)
	if err != nil || !loadedA.Equal(epochA) {
		t.Fatalf("Zone A Load(1) = (%+v, %v), want %+v", loadedA, err, epochA)
	}
	loadedB, err := store.Load(zoneB, epochB.ID)
	if err != nil || !loadedB.Equal(epochB) {
		t.Fatalf("Zone B Load(1) = (%+v, %v), want %+v", loadedB, err, epochB)
	}

	if _, err := store.Current(t.Context()); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("unscoped Current() error = %v, want %v", err, zone.ErrContextRequired)
	}
	if _, err := store.Load(t.Context(), 1); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("unscoped Load() error = %v, want %v", err, zone.ErrContextRequired)
	}
}
