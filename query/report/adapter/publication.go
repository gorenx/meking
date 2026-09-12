package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

// ReportSets loads the exact immutable ReportSet named by Query's fixed Epoch.
type ReportSets interface {
	LoadReportSet(ctx context.Context, id communityreport.ReportSetID) (communityreport.ReportSet, error)
}

// CommunitySets loads the exact immutable hierarchy bound by a ReportSet.
type CommunitySets interface {
	Load(ctx context.Context, id community.CommunitySetID) (community.CommunitySet, error)
}

// Reports is the Community Report application capability that loads exact
// immutable Reports in caller order.
type Reports interface {
	Reports(ctx context.Context, ids []communityreport.ID) ([]communityreport.Report, error)
}

// PublicationReader maps an exact Community ReportSet, hierarchy, and Reports
// into the complete publication projection required by Report Query.
type PublicationReader struct {
	reportSets    ReportSets
	communitySets CommunitySets
	reports       Reports
}

var _ queryreport.ReportPublicationReader = (*PublicationReader)(nil)

// NewPublicationReader creates the Report Query adapter from Community's exact
// ReportSet, CommunitySet, and immutable Report application APIs.
func NewPublicationReader(
	reportSets ReportSets,
	communitySets CommunitySets,
	reports Reports,
) (*PublicationReader, error) {
	if reportSets == nil {
		return nil, errors.New("create Report publication reader: ReportSet Catalog is required")
	}
	if communitySets == nil {
		return nil, errors.New("create Report publication reader: CommunitySet Catalog is required")
	}
	if reports == nil {
		return nil, errors.New("create Report publication reader: Report Archive is required")
	}
	return &PublicationReader{
		reportSets: reportSets, communitySets: communitySets, reports: reports,
	}, nil
}

// ReportPublication loads the requested ReportSet, its exact CommunitySet, and
// every immutable Report in deterministic Community order.
func (r *PublicationReader) ReportPublication(
	ctx context.Context,
	reportSetID string,
) (queryreport.ReportPublication, error) {
	if r == nil || r.reportSets == nil || r.communitySets == nil || r.reports == nil {
		return queryreport.ReportPublication{}, errors.New("Report publication reader is not configured")
	}
	reportSet, err := r.reportSets.LoadReportSet(ctx, communityreport.ReportSetID(reportSetID))
	if err != nil {
		return queryreport.ReportPublication{}, err
	}
	communitySet, err := r.communitySets.Load(ctx, reportSet.CommunitySetID)
	if err != nil {
		return queryreport.ReportPublication{}, err
	}
	ids := make([]communityreport.ID, len(communitySet.Communities))
	for index, membership := range communitySet.Communities {
		ids[index] = reportSet.Reports[membership.ID]
	}
	reports, err := r.reports.Reports(ctx, ids)
	if err != nil {
		return queryreport.ReportPublication{}, err
	}
	result := queryreport.ReportPublication{
		ReportSetID: string(reportSet.ID), CommunitySetID: string(communitySet.ID),
		CorporaID:   reportSet.CorporaID,
		Communities: make([]queryreport.PublishedCommunity, len(communitySet.Communities)),
		Reports:     make([]queryreport.PublishedReport, len(reports)),
	}
	for index, membership := range communitySet.Communities {
		result.Communities[index] = communityProjection(membership)
		result.Reports[index] = reportProjection(reports[index])
	}
	return result, nil
}

func communityProjection(membership community.Membership) queryreport.PublishedCommunity {
	var parentID *string
	if membership.ParentID != nil {
		value := string(*membership.ParentID)
		parentID = &value
	}
	return queryreport.PublishedCommunity{
		ID: string(membership.ID), Number: membership.Number, Level: membership.Level,
		ParentID: parentID, EntityIDs: append([]string(nil), membership.EntityIDs...),
	}
}

func reportProjection(report communityreport.Report) queryreport.PublishedReport {
	entities := make([]querybase.KnowledgeReference, len(report.EntitySources))
	for index, source := range report.EntitySources {
		entities[index] = querybase.KnowledgeReference{ID: source.ID, Version: source.Version}
	}
	relations := make([]querybase.KnowledgeReference, len(report.RelationSources))
	for index, source := range report.RelationSources {
		relations[index] = querybase.KnowledgeReference{ID: source.ID, Version: source.Version}
	}
	claims := make([]querybase.ClaimReference, len(report.ClaimSources))
	for index, source := range report.ClaimSources {
		claims[index] = querybase.ClaimReference{
			ID: source.ID, Version: source.Version, EvidenceIndex: source.EvidenceIndex,
		}
	}
	findings := make([]queryreport.ReportFinding, len(report.Findings))
	for index, finding := range report.Findings {
		findings[index] = queryreport.ReportFinding{
			Summary: finding.Summary, Explanation: finding.Explanation,
		}
	}
	return queryreport.PublishedReport{
		ID: string(report.ID), CommunityID: string(report.CommunityID), Period: report.Period,
		Title: report.Title, Summary: report.Summary, Findings: findings, Rank: report.Rank,
		RatingExplanation: report.RatingExplanation, FullContent: report.FullContent,
		Sources: queryreport.ReportSources{
			Entities: entities, Relations: relations, Claims: claims,
			TextUnitIDs: append([]string(nil), report.TextUnitIDs...),
		},
	}
}
