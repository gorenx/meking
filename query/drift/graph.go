package drift

import (
	"strings"

	querybase "github.com/memoria-space/meking/query"
)

type branchGraph struct {
	result  *TraversalResult
	byQuery map[string]int
}

func newBranchGraph(result *TraversalResult, maxDepth int) *branchGraph {
	graph := &branchGraph{
		result: result,
		byQuery: map[string]int{
			result.Primer.Question: -1,
		},
	}
	graph.add(result.Primer.Question, 0, result.Primer.FollowUpQueries, maxDepth)
	return graph
}

func (g *branchGraph) add(parent string, parentDepth int, questions []string, maxDepth int) {
	if parentDepth >= maxDepth {
		return
	}
	for _, raw := range questions {
		question := strings.TrimSpace(raw)
		if question == "" {
			continue
		}
		if _, exists := g.byQuery[question]; !exists {
			branch := Branch{Question: question, Depth: parentDepth + 1, Status: BranchPending}
			g.byQuery[question] = len(g.result.Branches)
			g.result.Branches = append(g.result.Branches, branch)
		}
		g.result.Edges = append(g.result.Edges, Edge{Parent: parent, Child: question})
	}
}

func (g *branchGraph) pending(limit int) []int {
	result := make([]int, 0, limit)
	for index := range g.result.Branches {
		if g.result.Branches[index].Status != BranchPending {
			continue
		}
		result = append(result, index)
		if len(result) == limit {
			break
		}
	}
	return result
}

func (g *branchGraph) failPending(failure *querybase.Failure) {
	for index := range g.result.Branches {
		if g.result.Branches[index].Status == BranchPending {
			g.result.Branches[index].Status = BranchFailed
			g.result.Branches[index].Failure = failure
		}
	}
}

func (g *branchGraph) discardPending() {
	discarded := make(map[string]struct{})
	branches := g.result.Branches[:0]
	for _, branch := range g.result.Branches {
		if branch.Status == BranchPending {
			discarded[branch.Question] = struct{}{}
			continue
		}
		branches = append(branches, branch)
	}
	g.result.Branches = branches
	if len(discarded) == 0 {
		return
	}
	edges := g.result.Edges[:0]
	for _, edge := range g.result.Edges {
		if _, drop := discarded[edge.Child]; !drop {
			edges = append(edges, edge)
		}
	}
	g.result.Edges = edges
}
