// Package knowledge browses active Knowledge objects at one Epoch-fixed
// Knowledge boundary.
package knowledge

import (
	"context"
	"errors"

	domain "github.com/memoria-space/meking/knowledge"
	querysource "github.com/memoria-space/meking/query/source"
)

const (
	DefaultPageSize = 50
	MaximumPageSize = 200
)

var (
	ErrInvalidRequest   = errors.New("invalid Knowledge browse request")
	ErrNoPublication    = errors.New("no Knowledge publication")
	ErrEntityNotFound   = errors.New("Knowledge Entity not found")
	ErrRelationNotFound = errors.New("Knowledge Relation not found")
)

// Publication identifies the immutable Query boundary used by one response.
type Publication struct {
	EpochID     int64
	CorporaID   string
	ReportSetID string
}

type PageRequest struct {
	After string
	Limit int
}

type Entity struct {
	ID            string
	Version       uint64
	Title         string
	Type          string
	Aliases       []string
	Description   string
	Degree        int
	EvidenceCount int
	TextUnitIDs   []string
}

type EntityPage struct {
	Publication
	Items     []Entity
	NextAfter string
	HasMore   bool
}

type Relation struct {
	ID             string
	Version        uint64
	SourceEntityID string
	TargetEntityID string
	Description    string
	Weight         float64
	CombinedDegree int
	EvidenceCount  int
	TextUnitIDs    []string
}

type RelationPage struct {
	Publication
	Items     []Relation
	NextAfter string
	HasMore   bool
}

type ClaimEvidence struct {
	TextUnitID  string
	SubjectText string
	ObjectText  string
	Status      string
	StartDate   string
	EndDate     string
	Description string
	SourceText  string
}

type Claim struct {
	ID        string
	Version   uint64
	SubjectID string
	Type      string
	Evidence  []ClaimEvidence
}

type ClaimPage struct {
	Publication
	Items     []Claim
	NextAfter string
	HasMore   bool
}

type EntityDetail struct {
	Publication
	Entity    Entity
	Relations []Relation
	Neighbors []Entity
	Claims    []Claim
	TextUnits []querysource.TextUnitSource
}

type RelationDetail struct {
	Publication
	Relation  Relation
	Endpoints []Entity
	Claims    []Claim
	TextUnits []querysource.TextUnitSource
}

// Page preserves the provider-owned exclusive cursor. HasMore must not be
// inferred from the number of returned items.
type Page[T any] struct {
	Items     []T
	NextAfter string
	HasMore   bool
}

// View remains fixed at one exact Knowledge Version set until Close.
type View interface {
	Entities(ctx context.Context, after string, limit int) (Page[Entity], error)
	Relations(ctx context.Context, after string, limit int) (Page[Relation], error)
	Claims(ctx context.Context, after string, limit int) (Page[Claim], error)
	Close() error
}

type Reader interface {
	Open(ctx context.Context, versions domain.Manifest) (View, error)
}
