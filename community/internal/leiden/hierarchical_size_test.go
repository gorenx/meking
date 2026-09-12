package leiden

import (
	"context"
	"errors"
	"testing"
)

func TestHierarchicalLeidenLeavesUnsplittableThresholdCommunityFinal(t *testing.T) {
	edges := []Edge{
		{From: 0, To: 1, Weight: 1},
		{From: 1, To: 2, Weight: 1},
		{From: 0, To: 2, Weight: 1},
	}
	opts := DefaultOptions()
	opts.Objective = ObjectiveModularity
	opts.Resolution = 1
	opts.Randomness = 0.001
	opts.Seed = 42

	result, err := HierarchicalLeiden(context.Background(), 3, edges, HierarchicalOptions{
		Options:        opts,
		MaxClusterSize: 3,
	})
	if err != nil {
		t.Fatalf("hierarchical Leiden: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("memberships = %d, want 3", len(result))
	}
	for _, membership := range result {
		if membership.Level != 0 || membership.ParentCluster != -1 || !membership.IsFinalCluster {
			t.Fatalf("unexpected membership: %+v", membership)
		}
	}
}

func TestHierarchicalLeidenRecursivelySplitsRing(t *testing.T) {
	const n = 24
	edges := make([]Edge, 0, n)
	for node := 0; node < n; node++ {
		edges = append(edges, Edge{From: node, To: (node + 1) % n, Weight: 1})
	}
	opts := DefaultOptions()
	opts.Objective = ObjectiveModularity
	opts.Resolution = 1
	opts.Randomness = 0.001
	opts.Seed = 42

	result, err := HierarchicalLeiden(context.Background(), n, edges, HierarchicalOptions{
		Options:        opts,
		MaxClusterSize: 4,
	})
	if err != nil {
		t.Fatalf("hierarchical Leiden: %v", err)
	}
	rootCount := 0
	childCount := 0
	for _, membership := range result {
		if membership.Level == 0 {
			rootCount++
		} else {
			childCount++
			if membership.ParentCluster < 0 {
				t.Fatalf("child has no parent: %+v", membership)
			}
		}
	}
	if rootCount != n {
		t.Fatalf("root memberships = %d, want %d", rootCount, n)
	}
	if childCount == 0 {
		t.Fatalf("ring produced no recursive memberships: %+v", result)
	}
}

func TestHierarchicalLeidenHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := HierarchicalLeiden(ctx, 2, []Edge{{From: 0, To: 1, Weight: 1}}, HierarchicalOptions{
		Options:        DefaultOptions(),
		MaxClusterSize: 10,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}
