package leiden

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestLeidenObjectiveChangesOptimization(t *testing.T) {
	edges := []Edge{
		{From: 0, To: 1, Weight: 1},
		{From: 1, To: 2, Weight: 1},
		{From: 0, To: 2, Weight: 1},
	}

	cpm := DefaultOptions()
	cpm.Resolution = 1
	cpm.Randomness = 0
	cpm.Seed = 42
	cpmResult, err := Leiden(context.Background(), 3, edges, cpm)
	if err != nil {
		t.Fatalf("CPM Leiden: %v", err)
	}
	if cpmResult.NumClusters != 3 {
		t.Fatalf("CPM clusters = %d, want 3", cpmResult.NumClusters)
	}

	modularity := cpm
	modularity.Objective = ObjectiveModularity
	modularityResult, err := Leiden(context.Background(), 3, edges, modularity)
	if err != nil {
		t.Fatalf("modularity Leiden: %v", err)
	}
	if modularityResult.NumClusters != 1 {
		t.Fatalf("modularity clusters = %d, want 1", modularityResult.NumClusters)
	}
}

func TestModularityObjectiveUsesDegreeMassAndAdjustedResolution(t *testing.T) {
	edges := []Edge{
		{From: 0, To: 1, Weight: 1},
		{From: 1, To: 2, Weight: 1},
		{From: 0, To: 2, Weight: 1},
	}
	opts := DefaultOptions()
	opts.Objective = ObjectiveModularity
	opts.Resolution = 1

	net, quality, err := prepareObjective(3, edges, opts)
	if err != nil {
		t.Fatalf("prepare objective: %v", err)
	}
	for node := 0; node < 3; node++ {
		if got := net.NodeWeight(node); got != 2 {
			t.Errorf("node %d mass = %g, want 2", node, got)
		}
	}
	if got, want := quality.resolution(), 1.0/6.0; math.Abs(got-want) > 1e-12 {
		t.Fatalf("adjusted resolution = %.15g, want %.15g", got, want)
	}
}

func TestModularityObjectiveIsInvariantUnderAggregation(t *testing.T) {
	edges := []Edge{
		{From: 0, To: 1, Weight: 2},
		{From: 1, To: 2, Weight: 2},
		{From: 0, To: 2, Weight: 2},
		{From: 2, To: 3, Weight: 0.1},
		{From: 3, To: 4, Weight: 2},
		{From: 4, To: 5, Weight: 2},
		{From: 3, To: 5, Weight: 2},
	}
	opts := DefaultOptions()
	opts.Objective = ObjectiveModularity
	opts.Resolution = 1
	net, quality, err := prepareObjective(6, edges, opts)
	if err != nil {
		t.Fatalf("prepare objective: %v", err)
	}
	parent, err := NewClusteringFromAssignment([]int{0, 0, 0, 1, 1, 1})
	if err != nil {
		t.Fatalf("parent clustering: %v", err)
	}
	refined, err := NewClusteringFromAssignment([]int{0, 0, 1, 2, 3, 3})
	if err != nil {
		t.Fatalf("refined clustering: %v", err)
	}

	aggregated, initial, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregate network: %v", err)
	}
	if got, want := quality.value(aggregated, initial), quality.value(net, parent); math.Abs(got-want) > 1e-12 {
		t.Fatalf("aggregated quality = %.15g, want %.15g", got, want)
	}
}

func TestModularityRejectsCallerNodeWeights(t *testing.T) {
	opts := DefaultOptions()
	opts.Objective = ObjectiveModularity
	opts.NodeWeights = []float64{1, 1}
	_, err := Leiden(context.Background(), 2, []Edge{{From: 0, To: 1, Weight: 1}}, opts)
	if !errors.Is(err, ErrNodeWeightsWithModularity) {
		t.Fatalf("error = %v, want %v", err, ErrNodeWeightsWithModularity)
	}
}
