package leiden

import "fmt"

// Objective selects the quality function optimized by Leiden.
type Objective uint8

const (
	// ObjectiveCPM optimizes the Constant Potts Model with explicit node masses.
	ObjectiveCPM Objective = iota

	// ObjectiveModularity optimizes generalized Newman-Girvan modularity.
	ObjectiveModularity
)

func (o Objective) validate() error {
	switch o {
	case ObjectiveCPM, ObjectiveModularity:
		return nil
	default:
		return fmt.Errorf("%w: %d", ErrInvalidObjective, o)
	}
}

// prepareObjective builds the network representation and quality function
// required by the selected objective. Modularity is optimized through the
// same linear quality increment as CPM, but each node mass is its weighted
// degree and gamma is scaled by 1/(2m). Summing those masses during graph
// aggregation preserves the objective across coarsening levels.
func prepareObjective(nNodes int, edges []Edge, opts Options) (*CompactNetwork, qualityFunction, error) {
	if err := opts.Objective.validate(); err != nil {
		return nil, nil, err
	}

	if opts.Objective == ObjectiveCPM {
		net, err := NewCompactNetworkWithNodeWeights(nNodes, opts.NodeWeights, edges)
		if err != nil {
			return nil, nil, err
		}
		return net, cpmQuality{Gamma: opts.Resolution}, nil
	}

	if opts.NodeWeights != nil {
		return nil, nil, ErrNodeWeightsWithModularity
	}

	base, err := NewCompactNetwork(nNodes, edges)
	if err != nil {
		return nil, nil, err
	}

	degreeMasses := make([]float64, nNodes)
	for node := 0; node < nNodes; node++ {
		neighbors := base.Neighbors(node)
		weights := base.NeighborWeights(node)
		for i, neighbor := range neighbors {
			if neighbor != node {
				degreeMasses[node] += weights[i]
			}
		}
	}

	net, err := NewCompactNetworkWithNodeWeights(nNodes, degreeMasses, edges)
	if err != nil {
		return nil, nil, err
	}

	adjustedResolution := 0.0
	if base.TotalEdgeWeight() > 0 {
		adjustedResolution = opts.Resolution / (2 * base.TotalEdgeWeight())
	}
	return net, cpmQuality{Gamma: adjustedResolution}, nil
}
