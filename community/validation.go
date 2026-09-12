package community

import "fmt"

// Validate checks identity, parentage, membership containment, final coverage,
// and size-state invariants for a generated hierarchy.
func (h Hierarchy) Validate() error {
	if len(h.Communities) == 0 {
		return fmt.Errorf("%w: no communities", ErrInvalidHierarchy)
	}

	byID := make(map[int]Community, len(h.Communities))
	children := make(map[int]int)
	rootNodes := make(map[string]struct{})
	finalNodes := make(map[string]int)
	for _, current := range h.Communities {
		if _, found := byID[current.ID]; found {
			return fmt.Errorf("%w: duplicate community %d", ErrInvalidHierarchy, current.ID)
		}
		if len(current.Nodes) == 0 {
			return fmt.Errorf("%w: community %d has no nodes", ErrInvalidHierarchy, current.ID)
		}
		byID[current.ID] = current
		if current.Level == 0 {
			if current.ParentID != -1 {
				return fmt.Errorf("%w: root community %d has parent %d", ErrInvalidHierarchy, current.ID, current.ParentID)
			}
			for _, node := range current.Nodes {
				rootNodes[node] = struct{}{}
			}
		} else if current.ParentID < 0 {
			return fmt.Errorf("%w: community %d has no parent", ErrInvalidHierarchy, current.ID)
		}
		if current.Final {
			for _, node := range current.Nodes {
				finalNodes[node]++
			}
		}
	}

	for _, current := range h.Communities {
		if current.Level == 0 {
			continue
		}
		parent, found := byID[current.ParentID]
		if !found {
			return fmt.Errorf("%w: community %d references missing parent %d", ErrInvalidHierarchy, current.ID, current.ParentID)
		}
		if parent.Level >= current.Level {
			return fmt.Errorf("%w: community %d does not descend from an earlier level", ErrInvalidHierarchy, current.ID)
		}
		if !nodesContained(current.Nodes, parent.Nodes) {
			return fmt.Errorf("%w: community %d has nodes outside parent %d", ErrInvalidHierarchy, current.ID, parent.ID)
		}
		children[parent.ID]++
	}

	for _, current := range h.Communities {
		if current.Final && children[current.ID] > 0 {
			return fmt.Errorf("%w: final community %d has children", ErrInvalidHierarchy, current.ID)
		}
		if !current.Final && children[current.ID] == 0 {
			return fmt.Errorf("%w: non-final community %d has no children", ErrInvalidHierarchy, current.ID)
		}
	}
	for node := range rootNodes {
		if finalNodes[node] != 1 {
			return fmt.Errorf("%w: node %q has %d final memberships", ErrInvalidHierarchy, node, finalNodes[node])
		}
	}
	return nil
}

func nodesContained(nodes, parentNodes []string) bool {
	parent := make(map[string]struct{}, len(parentNodes))
	for _, node := range parentNodes {
		parent[node] = struct{}{}
	}
	for _, node := range nodes {
		if _, found := parent[node]; !found {
			return false
		}
	}
	return true
}
