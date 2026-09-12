package community

import (
	"sort"
)

type weightedEdge struct {
	Source string
	Target string
	Weight float64
}

type edgeKey struct {
	source string
	target string
}

func prepareEdges(input []weightedEdge, useLCC bool) []weightedEdge {
	rows := normalizeDirectionKeepLast(input)
	if useLCC {
		rows = stableLargestConnectedComponent(rows)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].Target < rows[j].Target
	})
	return rows
}

func normalizeDirectionKeepLast(input []weightedEdge) []weightedEdge {
	rows := make([]weightedEdge, len(input))
	last := make(map[edgeKey]int, len(input))
	for i, edge := range input {
		source, target := orderedPair(edge.Source, edge.Target)
		rows[i] = weightedEdge{Source: source, Target: target, Weight: edge.Weight}
		last[edgeKey{source: source, target: target}] = i
	}

	result := make([]weightedEdge, 0, len(last))
	for index, row := range rows {
		key := edgeKey{source: row.Source, target: row.Target}
		if last[key] == index {
			result = append(result, row)
		}
	}
	return result
}

func stableLargestConnectedComponent(input []weightedEdge) []weightedEdge {
	if len(input) == 0 {
		return nil
	}

	normalized := make([]weightedEdge, len(input))
	for i, edge := range input {
		normalized[i] = weightedEdge{
			Source: edge.Source,
			Target: edge.Target,
			Weight: edge.Weight,
		}
	}

	largest := largestComponent(normalized)
	filtered := make([]weightedEdge, 0, len(normalized))
	seen := make(map[edgeKey]struct{})
	for _, edge := range normalized {
		if _, found := largest[edge.Source]; !found {
			continue
		}
		if _, found := largest[edge.Target]; !found {
			continue
		}
		source, target := orderedPair(edge.Source, edge.Target)
		key := edgeKey{source: source, target: target}
		if _, found := seen[key]; found {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, weightedEdge{Source: source, Target: target, Weight: edge.Weight})
	}
	return filtered
}

func orderedPair(left, right string) (string, string) {
	if left <= right {
		return left, right
	}
	return right, left
}

func largestComponent(edges []weightedEdge) map[string]struct{} {
	orderedNodes := make([]string, 0, len(edges)*2)
	seenNodes := make(map[string]struct{}, len(edges)*2)
	for _, edge := range edges {
		if _, found := seenNodes[edge.Source]; !found {
			seenNodes[edge.Source] = struct{}{}
			orderedNodes = append(orderedNodes, edge.Source)
		}
	}
	for _, edge := range edges {
		if _, found := seenNodes[edge.Target]; !found {
			seenNodes[edge.Target] = struct{}{}
			orderedNodes = append(orderedNodes, edge.Target)
		}
	}

	parent := make(map[string]string, len(orderedNodes))
	for _, node := range orderedNodes {
		parent[node] = node
	}
	var find func(string) string
	find = func(node string) string {
		root := node
		for parent[root] != root {
			root = parent[root]
		}
		for parent[node] != node {
			next := parent[node]
			parent[node] = root
			node = next
		}
		return root
	}
	for _, edge := range edges {
		left := find(edge.Source)
		right := find(edge.Target)
		if left != right {
			parent[left] = right
		}
	}

	components := make(map[string][]string)
	componentOrder := make([]string, 0)
	for _, node := range orderedNodes {
		root := find(node)
		if _, found := components[root]; !found {
			componentOrder = append(componentOrder, root)
		}
		components[root] = append(components[root], node)
	}

	bestRoot := componentOrder[0]
	for _, root := range componentOrder[1:] {
		if len(components[root]) > len(components[bestRoot]) {
			bestRoot = root
		}
	}
	result := make(map[string]struct{}, len(components[bestRoot]))
	for _, node := range components[bestRoot] {
		result[node] = struct{}{}
	}
	return result
}
