package graph

import (
	"errors"
)

// Catalog exposes the current formal Knowledge graph and the independently
// derived current Community Structure without coupling their lifecycles.
type Catalog struct {
	knowledge  KnowledgeReader
	structures StructureReader
}

func NewCatalog(
	knowledge KnowledgeReader,
	structures StructureReader,
) (*Catalog, error) {
	if knowledge == nil {
		return nil, errors.New("create graph Catalog: Knowledge reader is required")
	}
	if structures == nil {
		return nil, errors.New("create graph Catalog: Community Structure reader is required")
	}
	return &Catalog{knowledge: knowledge, structures: structures}, nil
}
