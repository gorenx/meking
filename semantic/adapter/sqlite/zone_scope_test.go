package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/memoria-space/meking/semantic"
	"github.com/memoria-space/meking/zone"
)

func TestSemanticStorePartitionsNamespaceMembershipByZone(t *testing.T) {
	store, _ := openStore(t, filepath.Join(t.TempDir(), "semantic.sqlite"))
	zoneA := testZoneContext(t, "10000000-0000-4000-8000-000000000001")
	zoneB := testZoneContext(t, "20000000-0000-4000-8000-000000000002")
	const namespace semantic.Namespace = "1"
	const vectorA = "10000000-0000-4000-8000-000000000003"
	const vectorB = "20000000-0000-4000-8000-000000000003"

	if err := store.Add(zoneA, namespace, []semantic.Vector{{ID: vectorA, Values: []float64{1, 0}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(zoneB, namespace, []semantic.Vector{{ID: vectorB, Values: []float64{0, 1}}}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		ctx      context.Context
		vectorID string
		want     []float64
	}{
		{ctx: zoneA, vectorID: vectorA, want: []float64{1, 0}},
		{ctx: zoneB, vectorID: vectorB, want: []float64{0, 1}},
	} {
		vectors, err := store.Lookup(test.ctx, []string{test.vectorID})
		if err != nil || len(vectors) != 1 || vectors[0].ID != test.vectorID ||
			vectors[0].Values[0] != test.want[0] || vectors[0].Values[1] != test.want[1] {
			t.Fatalf("Lookup() = %#v, %v", vectors, err)
		}
		reader, err := store.Open(test.ctx, namespace)
		if err != nil {
			t.Fatal(err)
		}
		ids, err := reader.IDs(test.ctx, "", 10)
		if closeErr := reader.Close(); err == nil {
			err = closeErr
		}
		if err != nil || len(ids.Items) != 1 || ids.Items[0] != test.vectorID {
			t.Fatalf("IDs() = %#v, %v", ids, err)
		}
	}
	if err := store.Delete(zoneA, namespace); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(zoneA, namespace); !errors.Is(err, semantic.ErrNamespaceNotFound) {
		t.Fatalf("Open(deleted Zone A) error = %v", err)
	}
	reader, err := store.Open(zoneB, namespace)
	if err != nil {
		t.Fatalf("Open(Zone B) error = %v", err)
	}
	if _, err := reader.Search(zoneB, []float64{0, 1}, 1, map[string]string{
		"zone_id": "10000000-0000-4000-8000-000000000001",
	}); err == nil {
		t.Fatal("Search() accepted a Filter from another Zone")
	}
	_ = reader.Close()
}

func TestSemanticStoreRejectsMissingZoneContext(t *testing.T) {
	store, _ := openStore(t, filepath.Join(t.TempDir(), "semantic.sqlite"))
	if err := store.Add(context.Background(), "1", nil); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("Add() error = %v", err)
	}
	if _, err := store.Open(context.Background(), "1"); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Delete(context.Background(), "1"); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("Delete() error = %v", err)
	}
}
