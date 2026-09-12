package memory

import (
	"context"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

type Query struct {
	Title  string `json:"title"`
	Type   string `json:"type,omitempty"`
	Fuzzy  bool   `json:"fuzzy,omitempty"`
	Limits Limits `json:"limits"`
}

type Limits struct {
	Entities  int `json:"entities,omitempty"`
	Relations int `json:"relations,omitempty"`
	Claims    int `json:"claims,omitempty"`
	Evidence  int `json:"evidence,omitempty"`
}

type EntityMatch struct {
	Version  knowledge.KnowledgeVersion[knowledge.Entity]
	Score    float64
	Reason   string
	Evidence []provenance.EntityConfirmation
}

type RelationMatch struct {
	Version  knowledge.KnowledgeVersion[knowledge.Relation]
	Reason   string
	Evidence []provenance.RelationConfirmation
}

type ClaimMatch struct {
	Version  knowledge.KnowledgeVersion[knowledge.Claim]
	Reason   string
	Evidence []provenance.ClaimConfirmation
}

type Evidence struct {
	Reference provenance.Evidence
	Source    EvidenceSource
}

type EvidenceSource interface {
	evidenceSource()
}

type MessageEvidence struct {
	Occurrence message.Occurrence
}

func (MessageEvidence) evidenceSource() {}

type CorporaEvidence struct {
	Location corpus.TextUnitLocation
}

func (CorporaEvidence) evidenceSource() {}

type SearchResult struct {
	Entities  []EntityMatch
	Relations []RelationMatch
	Claims    []ClaimMatch
	Evidence  []Evidence
	Truncated bool
}

type EntityQuery struct {
	Text       string
	Candidates []knowledge.Reference[knowledge.EntityID]
	Limit      int
}

type EntitySimilarity struct {
	Reference knowledge.Reference[knowledge.EntityID]
	Score     float64
}

type EntityMatcher interface {
	MatchEntities(ctx context.Context, query EntityQuery) ([]EntitySimilarity, error)
}

type Corpora interface {
	TextUnitLocations(
		ctx context.Context,
		corporaID corpus.CorporaID,
		textUnitIDs []textunits.TextUnitID,
	) ([]corpus.TextUnitLocation, error)
}

type MessageReader interface {
	Read(ctx context.Context, ids []string) ([]message.Occurrence, error)
}

type SearchDependencies struct {
	Knowledge knowledge.ViewSource
	Evidence  provenance.EvidenceReader
	Entities  EntityMatcher
	Corpora   Corpora
	Messages  MessageReader
}
