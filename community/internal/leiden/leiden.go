package leiden

import (
	"context"
	"fmt"
	"math/rand"
)

// Options configures a [Leiden] or [CoarsenedLeiden] run.
//
// Construct an Options via [DefaultOptions] and modify the fields you need;
// constructing a literal directly is permitted but will not pick up future
// fields' defaults.
type Options struct {
	// Objective selects the quality function optimized by Leiden. The zero
	// value selects CPM.
	Objective Objective

	// Resolution is the gamma parameter of the selected quality function.
	// Larger values favour smaller, more numerous clusters.
	Resolution float64

	// Randomness is the θ parameter governing refinement-phase candidate
	// selection. θ ≤ 0 chooses the deterministic argmax of ΔH; θ > 0 samples
	// a candidate with probability ∝ exp(ΔH/θ). Larger θ produces more
	// varied sub-clusters; smaller θ sharpens toward argmax.
	Randomness float64

	// MaxIterations caps the number of coarsening iterations. The run also
	// terminates as soon as a local-move pass makes zero moves. A value of
	// zero means "use the package default" (currently 100). Negative values
	// are rejected.
	MaxIterations int

	// Seed initialises the deterministic pseudo-random number generator
	// used to shuffle node visit orders and (when Randomness > 0) to sample
	// refinement candidates. Two runs with the same Seed on the same input
	// produce identical output.
	Seed int64

	// NodeWeights, if non-nil, overrides the default unit weight for each
	// node. Length must equal nNodes. Weights must be non-negative.
	NodeWeights []float64
}

// DefaultOptions returns recommended Leiden settings:
//
//   - Resolution: 0.05
//   - Objective: CPM
//   - Randomness: 0.01
//   - MaxIterations: 100
//   - Seed: 0
//   - NodeWeights: nil (each node weight defaults to 1)
func DefaultOptions() Options {
	return Options{
		Objective:     ObjectiveCPM,
		Resolution:    0.05,
		Randomness:    0.01,
		MaxIterations: defaultMaxIterations,
		Seed:          0,
	}
}

// Result is the outcome of a single [Leiden] run.
type Result struct {
	// Partition[i] is the cluster ID assigned to node i. Cluster IDs are
	// contiguous in [0, NumClusters). The slice is owned by the caller.
	Partition []int

	// NumClusters is the number of distinct clusters in Partition.
	NumClusters int

	// Quality is the selected objective's score for Partition.
	Quality float64

	// Iterations is the number of coarsening iterations the algorithm
	// performed before converging or reaching MaxIterations.
	Iterations int
}

// LevelResult is the partition observed at one [CoarsenedLeiden] iteration.
type LevelResult struct {
	// Partition[i] is the cluster ID of original node i at this level.
	// The slice is owned by the caller.
	Partition []int

	// NumClusters is the number of distinct clusters in Partition.
	NumClusters int

	// Quality is the selected objective's score on the input network.
	Quality float64
}

// CoarseningResult is the outcome of a single [CoarsenedLeiden] run.
//
// Levels are ordered from finest (most clusters) to coarsest (fewest
// clusters). Every cluster at level k is a union of clusters at level k-1.
type CoarseningResult struct {
	// Levels is the per-coarsening-iteration partition of the original
	// nodes. Levels has at least one element.
	Levels []LevelResult

	// Final is the converged partition (the last element of Levels).
	Final Result
}

// defaultMaxIterations is used when [Options].MaxIterations is zero.
const defaultMaxIterations = 100

// Leiden runs the Leiden community-detection algorithm on a weighted,
// undirected graph and returns the final flat partition.
//
// The optimized objective is selected by Options.Objective.
//
// nNodes must be positive. edges may be empty, may contain self-loops, and
// may contain duplicate node pairs (whose weights sum). Edge weights and
// node weights must be non-negative.
//
// Reference: Traag, Waltman & van Eck, "From Louvain to Leiden: guaranteeing
// well-connected communities", Scientific Reports 9:5233 (2019).
func Leiden(ctx context.Context, nNodes int, edges []Edge, opts Options) (Result, error) {
	rng := rand.New(rand.NewSource(opts.Seed))
	res, err := runLeiden(ctx, nNodes, edges, opts, rng)
	if err != nil {
		return Result{}, err
	}
	return res.Final, nil
}

// CoarsenedLeiden runs the Leiden algorithm and returns the partition at
// every coarsening level, not only the final converged partition. Use this
// when the multi-level structure of the graph is informative (e.g. building
// a community dendrogram or selecting a resolution after the fact).
//
// CoarsenedLeiden has the same input contract as [Leiden]. It is distinct
// from [HierarchicalLeiden], which recursively splits communities by size.
func CoarsenedLeiden(ctx context.Context, nNodes int, edges []Edge, opts Options) (CoarseningResult, error) {
	rng := rand.New(rand.NewSource(opts.Seed))
	return runLeiden(ctx, nNodes, edges, opts, rng)
}

func runLeiden(ctx context.Context, nNodes int, edges []Edge, opts Options, rng *rand.Rand) (CoarseningResult, error) {
	if err := ctx.Err(); err != nil {
		return CoarseningResult{}, err
	}
	if opts.MaxIterations < 0 {
		return CoarseningResult{}, fmt.Errorf("leiden: MaxIterations must be non-negative, got %d", opts.MaxIterations)
	}
	maxIter := opts.MaxIterations
	if maxIter == 0 {
		maxIter = defaultMaxIterations
	}

	net, q, err := prepareObjective(nNodes, edges, opts)
	if err != nil {
		return CoarseningResult{}, err
	}

	parent, err := NewSingletonClustering(nNodes)
	if err != nil {
		return CoarseningResult{}, err
	}

	// origToCurrent[u] is the index of original node u in the current
	// (possibly aggregated) network. Initially the identity map.
	origToCurrent := make([]int, nNodes)
	for i := range origToCurrent {
		origToCurrent[i] = i
	}
	currentNet := net

	var levels []LevelResult
	for iter := 0; iter < maxIter; iter++ {
		moves, err := runLocalMoveContext(ctx, currentNet, parent, q, rng)
		if err != nil {
			return CoarseningResult{}, err
		}

		levelPart := projectToOriginal(origToCurrent, parent, nNodes)
		quality, err := scorePartition(nNodes, edges, opts, net, q, levelPart)
		if err != nil {
			return CoarseningResult{}, err
		}
		levels = append(levels, LevelResult{
			Partition:   levelPart.Assignment(),
			NumClusters: levelPart.NumClusters(),
			Quality:     quality,
		})

		if moves == 0 {
			break
		}

		refined, err := runRefinementContext(ctx, currentNet, parent, q, opts.Randomness, rng)
		if err != nil {
			return CoarseningResult{}, err
		}
		aggNet, aggInit, err := aggregateNetworkContext(ctx, currentNet, refined, parent)
		if err != nil {
			return CoarseningResult{}, err
		}

		for u := 0; u < nNodes; u++ {
			origToCurrent[u] = refined.assignment[origToCurrent[u]]
		}
		currentNet = aggNet
		parent = aggInit
	}

	last := levels[len(levels)-1]
	finalPart := make([]int, len(last.Partition))
	copy(finalPart, last.Partition)
	return CoarseningResult{
		Levels: levels,
		Final: Result{
			Partition:   finalPart,
			NumClusters: last.NumClusters,
			Quality:     last.Quality,
			Iterations:  len(levels),
		},
	}, nil
}

func scorePartition(nNodes int, edges []Edge, opts Options, net *CompactNetwork, q qualityFunction, part *Clustering) (float64, error) {
	if opts.Objective == ObjectiveModularity {
		return Modularity(nNodes, edges, part.assignment, opts.Resolution)
	}
	return q.value(net, part), nil
}

// projectToOriginal returns a normalized clustering of the original nodes
// induced by the current (possibly aggregated) partition.
//
// For each original node u, its cluster ID is the cluster ID of its current
// network node origToCurrent[u] under parent.
func projectToOriginal(origToCurrent []int, parent *Clustering, nNodes int) *Clustering {
	a := make([]int, nNodes)
	for u := 0; u < nNodes; u++ {
		a[u] = parent.assignment[origToCurrent[u]]
	}
	cl, _ := NewClusteringFromAssignment(a)
	cl.Normalize()
	return cl
}

// GroupBy returns a map from cluster ID to the sorted slice of node IDs in that cluster.
//
//	communities := leiden.GroupBy(result.Partition)
//	for id, nodes := range communities {
//	    fmt.Printf("cluster %d: %v\n", id, nodes)
//	}
func GroupBy(partition []int) map[int][]int {
	out := make(map[int][]int)
	for node, cluster := range partition {
		out[cluster] = append(out[cluster], node)
	}
	return out
}

// CommunityCount returns the number of distinct communities in the partition.
func CommunityCount(partition []int) int {
	seen := make(map[int]struct{}, len(partition))
	for _, c := range partition {
		seen[c] = struct{}{}
	}
	return len(seen)
}
