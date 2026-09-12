package leiden

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
)

// HierarchicalOptions configures recursive size-bounded Leiden clustering.
type HierarchicalOptions struct {
	// Options configures each Leiden run in the hierarchy. A single random
	// stream is shared by the root and all recursive subnetwork runs.
	Options Options

	// MaxClusterSize is the threshold at which a community is isolated and
	// clustered again. Communities exactly at the threshold are also retried,
	// so the threshold is inclusive.
	MaxClusterSize int
}

// DefaultHierarchicalOptions combines the application's hierarchy size with
// this package's default Leiden options.
func DefaultHierarchicalOptions() HierarchicalOptions {
	return HierarchicalOptions{
		Options:        DefaultOptions(),
		MaxClusterSize: 1000,
	}
}

// HierarchicalCluster records one node's membership at one hierarchy level.
// A node appears once at the root level and again each time its final
// community is successfully split.
type HierarchicalCluster struct {
	// Node is the zero-based input node ID.
	Node int

	// Cluster is unique across every level in this hierarchy.
	Cluster int

	// Level is zero for root communities and increases after each split wave.
	Level int

	// ParentCluster is -1 for root communities and otherwise identifies the
	// community that was split to produce Cluster.
	ParentCluster int

	// IsFinalCluster reports whether this is the node's finest membership.
	IsFinalCluster bool
}

// HierarchicalLeiden first partitions the complete graph and then repeatedly
// isolates communities whose size is at least MaxClusterSize. A community
// that Leiden cannot split remains final even when it reaches the threshold.
//
// This hierarchy represents recursive size refinement. It is different from
// [CoarsenedLeiden], which exposes intermediate aggregation iterations.
func HierarchicalLeiden(ctx context.Context, nNodes int, edges []Edge, opts HierarchicalOptions) ([]HierarchicalCluster, error) {
	if opts.MaxClusterSize <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidMaxClusterSize, opts.MaxClusterSize)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rng := rand.New(rand.NewSource(opts.Options.Seed))
	root, err := runLeiden(ctx, nNodes, edges, opts.Options, rng)
	if err != nil {
		return nil, err
	}

	currentPartition := append([]int(nil), root.Final.Partition...)
	rootGroups := groupedNodes(currentPartition)
	entries := make([]HierarchicalCluster, 0, nNodes)
	entryIndexes := make(map[int][]int, len(rootGroups))
	for _, group := range rootGroups {
		for _, node := range group.Nodes {
			entryIndexes[group.Cluster] = append(entryIndexes[group.Cluster], len(entries))
			entries = append(entries, HierarchicalCluster{
				Node:           node,
				Cluster:        group.Cluster,
				Level:          0,
				ParentCluster:  -1,
				IsFinalCluster: true,
			})
		}
	}

	nextClusterID := root.Final.NumClusters
	unsplittable := make(map[int]struct{})
	work := hierarchyWorkFor(rootGroups, opts.MaxClusterSize, unsplittable)
	level := 1

	for len(work) > 0 {
		for _, item := range work {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			subEdges := inducedEdges(item.Nodes, edges)
			subResult, err := runLeiden(ctx, len(item.Nodes), subEdges, opts.Options, rng)
			if err != nil {
				return nil, fmt.Errorf("split cluster %d: %w", item.Cluster, err)
			}
			if subResult.Final.NumClusters == 1 {
				unsplittable[item.Cluster] = struct{}{}
				continue
			}

			for _, index := range entryIndexes[item.Cluster] {
				entries[index].IsFinalCluster = false
			}

			localGroups := groupedNodes(subResult.Final.Partition)
			for _, localGroup := range localGroups {
				clusterID := nextClusterID + localGroup.Cluster
				for _, localNode := range localGroup.Nodes {
					originalNode := item.Nodes[localNode]
					currentPartition[originalNode] = clusterID
					entryIndexes[clusterID] = append(entryIndexes[clusterID], len(entries))
					entries = append(entries, HierarchicalCluster{
						Node:           originalNode,
						Cluster:        clusterID,
						Level:          level,
						ParentCluster:  item.Cluster,
						IsFinalCluster: true,
					})
				}
			}
			nextClusterID += subResult.Final.NumClusters
		}

		level++
		work = hierarchyWorkFor(groupedNodes(currentPartition), opts.MaxClusterSize, unsplittable)
	}

	return entries, nil
}

type nodeGroup struct {
	Cluster int
	Nodes   []int
}

type hierarchyWork struct {
	Cluster int
	Nodes   []int
}

func groupedNodes(partition []int) []nodeGroup {
	byCluster := make(map[int][]int)
	for node, cluster := range partition {
		byCluster[cluster] = append(byCluster[cluster], node)
	}

	clusterIDs := make([]int, 0, len(byCluster))
	for cluster := range byCluster {
		clusterIDs = append(clusterIDs, cluster)
	}
	sort.Ints(clusterIDs)

	groups := make([]nodeGroup, 0, len(clusterIDs))
	for _, cluster := range clusterIDs {
		groups = append(groups, nodeGroup{Cluster: cluster, Nodes: byCluster[cluster]})
	}
	return groups
}

func hierarchyWorkFor(groups []nodeGroup, maxClusterSize int, unsplittable map[int]struct{}) []hierarchyWork {
	work := make([]hierarchyWork, 0)
	for _, group := range groups {
		if len(group.Nodes) < maxClusterSize || len(group.Nodes) <= 1 {
			continue
		}
		if _, found := unsplittable[group.Cluster]; found {
			continue
		}
		work = append(work, hierarchyWork{
			Cluster: group.Cluster,
			Nodes:   append([]int(nil), group.Nodes...),
		})
	}
	return work
}

func inducedEdges(nodes []int, edges []Edge) []Edge {
	localByOriginal := make(map[int]int, len(nodes))
	for local, original := range nodes {
		localByOriginal[original] = local
	}

	result := make([]Edge, 0)
	for _, edge := range edges {
		from, hasFrom := localByOriginal[edge.From]
		to, hasTo := localByOriginal[edge.To]
		if !hasFrom || !hasTo {
			continue
		}
		result = append(result, Edge{From: from, To: to, Weight: edge.Weight})
	}
	return result
}
