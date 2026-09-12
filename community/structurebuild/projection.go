package structurebuild

import (
	"fmt"

	"github.com/memoria-space/meking/community"
)

func projectGraph(entities []Entity, relations []Relation) (community.Graph, error) {
	entityIDs := make(map[string]struct{}, len(entities))
	graph := community.Graph{
		Entities:  make([]community.EntityReference, len(entities)),
		Relations: make([]community.GraphRelation, len(relations)),
	}
	for index, entity := range entities {
		entityIDs[entity.ID] = struct{}{}
		graph.Entities[index] = community.EntityReference{
			ID:      entity.ID,
			Version: entity.Version,
		}
	}
	for index, relation := range relations {
		if _, found := entityIDs[relation.SourceEntityID]; !found {
			return community.Graph{}, fmt.Errorf(
				"project Community graph: Relation %q source Entity %q is not active",
				relation.ID,
				relation.SourceEntityID,
			)
		}
		if _, found := entityIDs[relation.TargetEntityID]; !found {
			return community.Graph{}, fmt.Errorf(
				"project Community graph: Relation %q target Entity %q is not active",
				relation.ID,
				relation.TargetEntityID,
			)
		}
		graph.Relations[index] = community.GraphRelation{
			Reference: community.RelationReference{
				ID:      relation.ID,
				Version: relation.Version,
			},
			SourceEntityID: relation.SourceEntityID,
			TargetEntityID: relation.TargetEntityID,
			Weight:         relation.Weight,
		}
	}
	if err := graph.Validate(); err != nil {
		return community.Graph{}, fmt.Errorf("project Community graph: %w", err)
	}
	return graph, nil
}
