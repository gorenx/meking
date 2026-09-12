package community

import (
	"context"
	"fmt"
	"sort"

	"github.com/memoria-space/meking/community/internal/leiden"
)

// Detect builds one immutable CommunitySet from exact Entity and Relation
// versions. Graph validation, hierarchical Leiden, and CommunitySet
// construction form one domain operation; persistence remains the caller's
// responsibility.
func Detect(
	ctx context.Context,
	graph Graph,
	cfg DetectConfig,
) (CommunitySet, error) {
	if err := graph.Validate(); err != nil {
		return CommunitySet{}, err
	}
	memberships, nodes, err := detectMemberships(ctx, graph.weightedEdges(), cfg)
	if err != nil {
		return CommunitySet{}, err
	}
	hierarchy := Hierarchy{
		Communities: collectCommunities(memberships, nodes, cfg.MaxClusterSize),
	}
	if err = hierarchy.Validate(); err != nil {
		return CommunitySet{}, err
	}
	return NewCommunitySet(
		hierarchy,
		graph.Entities,
		graph.relationReferences(),
		cfg,
	)
}

func detectMemberships(
	ctx context.Context,
	edges []weightedEdge,
	cfg DetectConfig,
) ([]leiden.HierarchicalCluster, []string, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%w: got %d", ErrInvalidMaxClusterSize, cfg.MaxClusterSize)
	}
	edges = prepareEdges(edges, cfg.UseLargestConnectedComponent)
	if len(edges) == 0 {
		return nil, nil, ErrEmptyGraph
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	nodes := make([]string, 0)
	nodeIDs := make(map[string]int)
	identify := func(node string) int {
		if id, found := nodeIDs[node]; found {
			return id
		}
		id := len(nodes)
		nodeIDs[node] = id
		nodes = append(nodes, node)
		return id
	}
	algorithmEdges := make([]leiden.Edge, 0, len(edges))
	for _, edge := range edges {
		algorithmEdges = append(algorithmEdges, leiden.Edge{
			From:   identify(edge.Source),
			To:     identify(edge.Target),
			Weight: edge.Weight,
		})
	}
	algorithmOptions := hierarchicalLeidenOptions(cfg.Seed)

	memberships, err := leiden.HierarchicalLeiden(
		ctx, len(nodes), algorithmEdges,
		leiden.HierarchicalOptions{
			Options:        algorithmOptions,
			MaxClusterSize: cfg.MaxClusterSize,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("detect communities: %w", err)
	}
	return memberships, nodes, nil
}

func hierarchicalLeidenOptions(seed int64) leiden.Options {
	algorithmOptions := leiden.DefaultOptions()
	algorithmOptions.Objective = leiden.ObjectiveModularity
	algorithmOptions.Resolution = 1.0
	algorithmOptions.Randomness = 0.001
	algorithmOptions.Seed = seed
	return algorithmOptions
}

func collectCommunities(memberships []leiden.HierarchicalCluster, nodes []string, maxClusterSize int) []Community {
	type key struct {
		level   int
		cluster int
	}
	byKey := make(map[key]*Community)
	keys := make([]key, 0)
	for _, membership := range memberships {
		groupKey := key{level: membership.Level, cluster: membership.Cluster}
		current, found := byKey[groupKey]
		if !found {
			current = &Community{
				ID:       membership.Cluster,
				Level:    membership.Level,
				ParentID: membership.ParentCluster,
				Final:    membership.IsFinalCluster,
			}
			byKey[groupKey] = current
			keys = append(keys, groupKey)
		}
		current.Nodes = append(current.Nodes, nodes[membership.Node])
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].level != keys[j].level {
			return keys[i].level < keys[j].level
		}
		return keys[i].cluster < keys[j].cluster
	})

	communities := make([]Community, 0, len(keys))
	for _, groupKey := range keys {
		current := *byKey[groupKey]
		current.Unsplittable = current.Final && len(current.Nodes) >= maxClusterSize
		communities = append(communities, current)
	}
	return communities
}
