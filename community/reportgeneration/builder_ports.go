package reportgeneration

import (
	"context"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/knowledge"
)

type KnowledgeSnapshot struct {
	Entities  []communityreport.Entity
	Relations []communityreport.Relation
	Claims    []communityreport.Claim
}

type KnowledgeReader interface {
	ReportSnapshot(context.Context, knowledge.Manifest) (KnowledgeSnapshot, error)
}

type CorpusSnapshot struct {
	ID          string
	TextUnitIDs []string
}

type CorpusReader interface {
	Corpora(context.Context, string) (CorpusSnapshot, error)
}

type Structures interface {
	LoadStructure(context.Context, community.StructureID) (community.Structure, error)
}

type CommunitySets interface {
	Load(context.Context, community.CommunitySetID) (community.CommunitySet, error)
}

type Generator interface {
	Generate(context.Context, communityreport.Input) ([]communityreport.Report, error)
}

type Reports interface {
	Save(context.Context, []communityreport.Report) error
	Reports(context.Context, []communityreport.ID) ([]communityreport.Report, error)
}

type ReportSets interface {
	SaveReportSet(context.Context, communityreport.ReportSet) error
	LoadReportSet(context.Context, communityreport.ReportSetID) (communityreport.ReportSet, error)
}

type Vectors interface {
	Build(
		context.Context,
		communityreport.ReportSet,
		[]communityreport.Report,
	) (int, error)
}
