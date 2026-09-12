package report

import (
	"sort"

	querybase "github.com/memoria-space/meking/query"
)

func reportSummary(
	report PublishedReport,
	membership PublishedCommunity,
) ReportSummary {
	return ReportSummary{
		ID:              report.ID,
		CommunityID:     report.CommunityID,
		CommunityNumber: membership.Number,
		Level:           membership.Level,
		Title:           report.Title,
		Summary:         report.Summary,
		Rank:            report.Rank,
		Period:          report.Period,
		Size:            len(membership.EntityIDs),
	}
}

func reportDetail(
	view View,
	report PublishedReport,
	membership PublishedCommunity,
) ReportDetail {
	return ReportDetail{
		EpochID:           view.EpochID,
		ReportSetID:       view.ReportSetID,
		CommunitySetID:    view.CommunitySetID,
		CorporaID:         view.CorporaID,
		ID:                report.ID,
		CommunityID:       report.CommunityID,
		CommunityNumber:   membership.Number,
		Level:             membership.Level,
		ParentID:          copyStringPointer(membership.ParentID),
		Children:          childCommunityIDs(view.Communities, membership.ID),
		Title:             report.Title,
		Summary:           report.Summary,
		FullContent:       report.FullContent,
		Rank:              report.Rank,
		RatingExplanation: report.RatingExplanation,
		Findings:          append([]ReportFinding(nil), report.Findings...),
		Period:            report.Period,
		Size:              len(membership.EntityIDs),
		Sources:           copyReportSources(report.Sources),
	}
}

func reportPosition(reports []PublishedReport, id string) (int, bool) {
	for index, report := range reports {
		if report.ID == id {
			return index, true
		}
	}
	return 0, false
}

func childCommunityIDs(communities []PublishedCommunity, parentID string) []string {
	children := make([]PublishedCommunity, 0)
	for _, membership := range communities {
		if membership.ParentID != nil && *membership.ParentID == parentID {
			children = append(children, membership)
		}
	}
	sort.Slice(children, func(left, right int) bool {
		return children[left].Number < children[right].Number
	})
	result := make([]string, len(children))
	for index, membership := range children {
		result[index] = membership.ID
	}
	return result
}

func copyReportSources(sources ReportSources) ReportSources {
	return ReportSources{
		Entities:    append([]querybase.KnowledgeReference(nil), sources.Entities...),
		Relations:   append([]querybase.KnowledgeReference(nil), sources.Relations...),
		Claims:      append([]querybase.ClaimReference(nil), sources.Claims...),
		TextUnitIDs: append([]string(nil), sources.TextUnitIDs...),
	}
}

func copyPublishedCommunities(values []PublishedCommunity) []PublishedCommunity {
	result := make([]PublishedCommunity, len(values))
	for index, value := range values {
		value.ParentID = copyStringPointer(value.ParentID)
		value.EntityIDs = append([]string(nil), value.EntityIDs...)
		result[index] = value
	}
	return result
}

func copyPublishedReports(values []PublishedReport) []PublishedReport {
	result := make([]PublishedReport, len(values))
	for index, value := range values {
		value.Findings = append([]ReportFinding(nil), value.Findings...)
		value.Sources = copyReportSources(value.Sources)
		result[index] = value
	}
	return result
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
