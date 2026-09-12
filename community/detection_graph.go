package community

import (
	"fmt"
	"math"

	"github.com/memoria-space/meking/internal/uuid"
)

// Graph is one complete, immutable-by-convention Community detection source.
// Community derivation projects it from active Entity and Relation versions in
// one Knowledge Read View, then derives the detector-only undirected projection
// so stored version references cannot drift from the detected graph.
type Graph struct {
	// Entities is every active Entity reference observed by the source Read View,
	// including isolated Entities. It is non-empty, strictly ordered by ID, and
	// contains no duplicate ID.
	Entities []EntityReference
	// Relations is every active Relation detection row observed by the same Read
	// View. It is strictly ordered by Reference.ID and contains no duplicate ID;
	// an empty slice is structurally valid but Detect returns ErrEmptyGraph.
	Relations []GraphRelation
}

// GraphRelation is the exact Relation version and minimum business data needed
// to derive one Community detector edge. Community copies all fields from the
// same active Knowledge Relation version; report content and evidence do not
// enter Community detection.
type GraphRelation struct {
	// Reference identifies the canonical Relation ID and positive Version that
	// supplied the endpoints and Weight.
	Reference RelationReference
	// SourceEntityID is the canonical directed source copied from the Knowledge
	// Relation version identified by Reference and must occur in Graph.Entities.
	SourceEntityID string
	// TargetEntityID is the canonical directed target copied from the Knowledge
	// Relation version identified by Reference and must occur in Graph.Entities.
	TargetEntityID string
	// Weight is the finite, non-negative clustering strength copied from that
	// Knowledge Relation version. Community preserves source direction here and
	// removes direction only inside Detect.
	Weight float64
}

// Validate checks one complete detection source without changing its order or
// values. It does not run Leiden or inspect Knowledge, Corpus, or persistence.
func (g Graph) Validate() error {
	if len(g.Entities) == 0 {
		return fmt.Errorf(
			"%w: at least one Entity reference is required",
			ErrInvalidDetectionGraph,
		)
	}
	entityIDs := make(map[string]struct{}, len(g.Entities))
	for index, reference := range g.Entities {
		if !uuid.IsCanonicalV4(reference.ID) {
			return fmt.Errorf(
				"%w: Entity ID %q is not a canonical lowercase UUID v4",
				ErrInvalidDetectionGraph,
				reference.ID,
			)
		}
		if err := validateDetectionVersion(reference.Version, "Entity", reference.ID); err != nil {
			return err
		}
		if index > 0 && g.Entities[index-1].ID >= reference.ID {
			return fmt.Errorf(
				"%w: Entity references must be strictly ordered by ID",
				ErrInvalidDetectionGraph,
			)
		}
		entityIDs[reference.ID] = struct{}{}
	}
	for index, relation := range g.Relations {
		reference := relation.Reference
		if !uuid.IsCanonicalV4(reference.ID) {
			return fmt.Errorf(
				"%w: Relation ID %q is not a canonical lowercase UUID v4",
				ErrInvalidDetectionGraph,
				reference.ID,
			)
		}
		if err := validateDetectionVersion(
			reference.Version,
			"Relation",
			reference.ID,
		); err != nil {
			return err
		}
		if index > 0 && g.Relations[index-1].Reference.ID >= reference.ID {
			return fmt.Errorf(
				"%w: Relation rows must be strictly ordered by ID",
				ErrInvalidDetectionGraph,
			)
		}
		if !uuid.IsCanonicalV4(relation.SourceEntityID) {
			return fmt.Errorf(
				"%w: Relation %q Source Entity ID %q is invalid",
				ErrInvalidDetectionGraph,
				reference.ID,
				relation.SourceEntityID,
			)
		}
		if _, found := entityIDs[relation.SourceEntityID]; !found {
			return fmt.Errorf(
				"%w: Relation %q Source Entity %q is absent",
				ErrInvalidDetectionGraph,
				reference.ID,
				relation.SourceEntityID,
			)
		}
		if !uuid.IsCanonicalV4(relation.TargetEntityID) {
			return fmt.Errorf(
				"%w: Relation %q Target Entity ID %q is invalid",
				ErrInvalidDetectionGraph,
				reference.ID,
				relation.TargetEntityID,
			)
		}
		if _, found := entityIDs[relation.TargetEntityID]; !found {
			return fmt.Errorf(
				"%w: Relation %q Target Entity %q is absent",
				ErrInvalidDetectionGraph,
				reference.ID,
				relation.TargetEntityID,
			)
		}
		if math.IsNaN(relation.Weight) ||
			math.IsInf(relation.Weight, 0) ||
			relation.Weight < 0 {
			return fmt.Errorf(
				"%w: Relation %q Weight must be finite and non-negative",
				ErrInvalidDetectionGraph,
				reference.ID,
			)
		}
	}
	return nil
}

func validateDetectionVersion(version uint64, object string, id string) error {
	if version == 0 || version > math.MaxInt64 {
		return fmt.Errorf(
			"%w: %s %q Version must fit a positive SQLite INTEGER",
			ErrInvalidDetectionGraph,
			object,
			id,
		)
	}
	return nil
}

func (g Graph) weightedEdges() []weightedEdge {
	result := make([]weightedEdge, len(g.Relations))
	for index, relation := range g.Relations {
		result[index] = weightedEdge{
			Source: relation.SourceEntityID,
			Target: relation.TargetEntityID,
			Weight: relation.Weight,
		}
	}
	return result
}

func (g Graph) relationReferences() []RelationReference {
	result := make([]RelationReference, len(g.Relations))
	for index, relation := range g.Relations {
		result[index] = relation.Reference
	}
	return result
}
