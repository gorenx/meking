// Package graph browses the formal Knowledge graph and Community Structure as
// two independent read models.
package graph

import "errors"

const (
	DefaultEntityLimit    = 50
	MaximumEntityLimit    = 200
	MaximumRelationLimit  = 1000
	DefaultCommunitySize  = 50
	MaximumCommunitySize  = 100
	knowledgeReadPageSize = 512
)

var (
	ErrInvalidRequest    = errors.New("invalid graph browse request")
	ErrNoStructure       = errors.New("no Community Structure")
	ErrEntityNotFound    = errors.New("graph entity not found")
	ErrCommunityNotFound = errors.New("graph community not found")
)

type Entity struct {
	ID            string
	Version       uint64
	Title         string
	Type          string
	Aliases       []string
	Description   string
	Degree        int
	TextUnitCount int
}

type Relation struct {
	ID             string
	Version        uint64
	SourceEntityID string
	TargetEntityID string
	Type           string
	Description    string
	Weight         float64
	CombinedDegree int
	TextUnitCount  int
}

type EntityGraphRequest struct {
	Query      string
	EntityType string
	Limit      int
}

type EntityGraph struct {
	Entities         []Entity
	Relations        []Relation
	MatchedEntities  int
	MatchedRelations int
	Truncated        bool
}

type NeighborhoodRequest struct {
	EntityID      string
	RelationLimit int
}

type Neighborhood struct {
	Center           Entity
	Entities         []Entity
	Relations        []Relation
	MatchedRelations int
	Truncated        bool
}

type CommunityRequest struct {
	Query    string
	ParentID *string
	Page     int
	PageSize int
}

type Community struct {
	ID          string
	Number      int
	Level       int
	ParentID    *string
	ChildCount  int
	EntityCount int
}

type CommunityPage struct {
	StructureID    string
	CommunitySetID string
	CorporaID      string
	Page           int
	PageSize       int
	Total          int
	Communities    []Community
}
