package leiden

import (
	"context"
	"math/rand"
	"sort"
)

// runLocalMove performs the Louvain-style local-moving phase: nodes are
// visited in shuffled order, and each is greedily reassigned to the
// neighbouring cluster offering the largest positive Δquality. When a node
// moves, its still-pending neighbours are re-enqueued. The loop terminates
// when the queue is drained — i.e. every node has been visited at least once
// since its last move and chose not to move.
//
// cl is mutated in place; its node-to-cluster assignments are updated but
// NumClusters never grows (local-move never creates new clusters).
//
// Returns the total number of moves performed.
//
// Pre-conditions:
//   - cl.NumNodes() == net.NumNodes()
//   - cl.NumClusters() correctly covers all cluster IDs that appear in the
//     assignment (the standard invariant of [NewClusteringFromAssignment]).
//
// rng is used only to shuffle the initial visit order; passing
// rand.New(rand.NewSource(seed)) gives deterministic runs.
func runLocalMove(net *CompactNetwork, cl *Clustering, q qualityFunction, rng *rand.Rand) int {
	moves, _ := runLocalMoveContext(context.Background(), net, cl, q, rng)
	return moves
}

func runLocalMoveContext(ctx context.Context, net *CompactNetwork, cl *Clustering, q qualityFunction, rng *rand.Rand) (int, error) {
	n := net.nNodes
	if n == 0 {
		return 0, nil
	}

	clusterMass := make([]float64, cl.nClusters)
	for u := 0; u < n; u++ {
		clusterMass[cl.assignment[u]] += q.nodeMass(net, u)
	}

	// Circular FIFO of pending nodes. inQueue dedups so the queue never
	// exceeds n elements.
	qbuf := make([]int, n)
	inQueue := make([]bool, n)
	perm := rng.Perm(n)
	copy(qbuf, perm)
	for _, p := range perm {
		inQueue[p] = true
	}
	head, qsize := 0, n

	// Scratch map: edge weight from u to each neighbouring cluster, keyed
	// by cluster ID. Reset per node by deletion (cheaper than reallocating
	// for sparse touches).
	edgeToCluster := make(map[int]float64)
	candidateClusters := make([]int, 0)

	moves := 0
	for qsize > 0 {
		if err := ctx.Err(); err != nil {
			return moves, err
		}
		u := qbuf[head]
		head = (head + 1) % n
		qsize--
		inQueue[u] = false

		for k := range edgeToCluster {
			delete(edgeToCluster, k)
		}
		nbrs := net.Neighbors(u)
		ws := net.NeighborWeights(u)
		for i, v := range nbrs {
			if v == u {
				continue
			}
			edgeToCluster[cl.assignment[v]] += ws[i]
		}

		curr := cl.assignment[u]
		wToCurr := edgeToCluster[curr]

		bestCluster := curr
		bestDelta := 0.0
		candidateClusters = candidateClusters[:0]
		for cluster := range edgeToCluster {
			candidateClusters = append(candidateClusters, cluster)
		}
		sort.Ints(candidateClusters)
		for _, c := range candidateClusters {
			if c == curr {
				continue
			}
			wToC := edgeToCluster[c]
			d := q.moveDelta(net, u, curr, c, wToCurr, wToC, clusterMass)
			if d > bestDelta {
				bestDelta = d
				bestCluster = c
			}
		}
		if bestCluster == curr {
			continue
		}

		mu := q.nodeMass(net, u)
		clusterMass[curr] -= mu
		clusterMass[bestCluster] += mu
		cl.assignment[u] = bestCluster
		moves++

		// Re-enqueue not-already-queued neighbours that are not already in
		// the new cluster: their best-move evaluation may have changed.
		for _, v := range nbrs {
			if v == u || inQueue[v] {
				continue
			}
			if cl.assignment[v] == bestCluster {
				continue
			}
			tail := (head + qsize) % n
			qbuf[tail] = v
			qsize++
			inQueue[v] = true
		}
	}
	return moves, nil
}
