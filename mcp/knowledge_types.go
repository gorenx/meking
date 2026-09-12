package mcp

import "github.com/memoria-space/meking/knowledge"

type EntitySource struct {
	Frequency int `json:"frequency"`
}

type EntityMemory struct {
	Content knowledge.EntityContent `json:"content"`
	Source  EntitySource            `json:"source"`
}

type RelationSource struct {
	Weight float64 `json:"weight"`
}

type RelationMemory struct {
	Content knowledge.RelationContent `json:"content"`
	Source  RelationSource            `json:"source"`
}

type ClaimSubject struct {
	Entity   *knowledge.EntityIdentity   `json:"entity,omitempty"`
	Relation *knowledge.RelationIdentity `json:"relation,omitempty"`
}

type ClaimSource struct {
	SubjectText string `json:"subject_text"`
	ObjectText  string `json:"object_text"`
	Status      string `json:"status"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	SourceText  string `json:"source_text"`
}

type ClaimMemory struct {
	Subject     ClaimSubject `json:"subject"`
	Type        string       `json:"type"`
	Description string       `json:"description"`
	Source      ClaimSource  `json:"source"`
}

type Subject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Claim struct {
	ID          string  `json:"id"`
	Subject     Subject `json:"subject"`
	Type        string  `json:"type"`
	Description string  `json:"description"`
}

type Evidence struct {
	ZoneID     string         `json:"zone_id"`
	TextUnitID string         `json:"text_unit_id"`
	Source     EvidenceSource `json:"source"`
}

type EvidenceSource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type EntityEvidence struct {
	SourceID  string     `json:"source_id"`
	Frequency int        `json:"frequency"`
	Evidence  []Evidence `json:"evidence"`
}

type RelationEvidence struct {
	SourceID string     `json:"source_id"`
	Weight   float64    `json:"weight"`
	Evidence []Evidence `json:"evidence"`
}

type ClaimRecord struct {
	SubjectText string     `json:"subject_text"`
	ObjectText  string     `json:"object_text"`
	Status      string     `json:"status"`
	StartDate   string     `json:"start_date"`
	EndDate     string     `json:"end_date"`
	SourceText  string     `json:"source_text"`
	Evidence    []Evidence `json:"evidence"`
}

type ClaimEvidence struct {
	SourceID string        `json:"source_id"`
	Records  []ClaimRecord `json:"records"`
}

type Version[T any] struct {
	Number  uint64 `json:"version"`
	Deleted bool   `json:"deleted"`
	Content T      `json:"content"`
}

type VersionChange struct {
	Version uint64 `json:"version"`
	Created bool   `json:"created"`
}

type ConflictReference struct {
	ContentHash string   `json:"content_hash"`
	SourceIDs   []string `json:"source_ids"`
}

type Alternative[T any, E any] struct {
	Revision  uint64            `json:"revision"`
	Content   T                 `json:"content"`
	Evidence  []E               `json:"evidence"`
	Reference ConflictReference `json:"reference"`
}

type Conflict[T any, E any] struct {
	ID           string              `json:"id"`
	BaseVersion  uint64              `json:"base_version"`
	Current      *Version[T]         `json:"current,omitempty"`
	Evidence     []E                 `json:"evidence"`
	Alternatives []Alternative[T, E] `json:"alternatives"`
}

type EntityConflict = Conflict[knowledge.Entity, EntityEvidence]
type RelationConflict = Conflict[knowledge.Relation, RelationEvidence]
type ClaimConflict = Conflict[Claim, ClaimEvidence]
