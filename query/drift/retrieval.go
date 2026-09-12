package drift

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

func (p *Primer) openReportSet(
	ctx context.Context,
	selected querybase.Epoch,
) (queryreport.View, string, error) {
	view, err := p.reports.Publication(ctx, selected)
	if err != nil {
		return queryreport.View{}, "", normalizePublicationFailure(err)
	}
	view = reportsThroughLevel(view, p.config.CommunityLevel)
	for _, report := range view.Reports {
		if strings.TrimSpace(report.FullContent) != "" {
			return view, report.FullContent, nil
		}
	}
	return queryreport.View{}, "", querybase.NewNoEvidenceFailure(
		fmt.Errorf("DRIFT ReportSet contains no Report at or below Community level %d", p.config.CommunityLevel),
	)
}

func reportsThroughLevel(view queryreport.View, level int) queryreport.View {
	communities := make([]queryreport.PublishedCommunity, 0, len(view.Communities))
	reports := make([]queryreport.PublishedReport, 0, len(view.Reports))
	for index, community := range view.Communities {
		if community.Level > level {
			continue
		}
		communities = append(communities, community)
		reports = append(reports, view.Reports[index])
	}
	view.Communities = communities
	view.Reports = reports
	return view
}

func (p *Primer) retrieveReports(
	ctx context.Context,
	selected querybase.Epoch,
	view queryreport.View,
	vector []float64,
) (result []SelectedReport, resultErr error) {
	reader, err := p.vectors.OpenReports(ctx, selected.ReportSetID)
	if err != nil {
		return nil, normalizePublicationFailure(err)
	}
	if reader == nil {
		return nil, querybase.NewPublicationIncompleteFailure(
			errors.New("DRIFT Report vector Namespace is missing"),
		)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				querybase.NewInternalFailure(closeErr),
			)
		}
	}()
	if reader.ReportSetID() != selected.ReportSetID ||
		strings.TrimSpace(reader.Model()) == "" || reader.Model() != p.embedder.Model() ||
		reader.Dimension() <= 0 {
		return nil, querybase.NewPublicationIncompleteFailure(
			errors.New("DRIFT Report vector Namespace does not match the fixed ReportSet and embedding model"),
		)
	}
	if len(vector) != reader.Dimension() {
		return nil, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
			"DRIFT hypothetical vector dimension %d does not match Report vector dimension %d",
			len(vector),
			reader.Dimension(),
		))
	}
	eligible := make([]string, len(view.Reports))
	for index, report := range view.Reports {
		eligible[index] = report.ID
	}
	matches, err := reader.Search(ctx, vector, p.config.Reports, eligible)
	if err != nil {
		return nil, normalizeFailure(err)
	}
	if len(matches) == 0 {
		return nil, querybase.NewNoEvidenceFailure(nil)
	}
	reports := make(map[string]queryreport.PublishedReport, len(view.Reports))
	for _, report := range view.Reports {
		reports[report.ID] = report
	}
	seen := make(map[string]struct{}, len(matches))
	result = make([]SelectedReport, len(matches))
	for index, match := range matches {
		if strings.TrimSpace(match.ReportID) == "" || match.ReportID != strings.TrimSpace(match.ReportID) ||
			math.IsNaN(match.Score) || math.IsInf(match.Score, 0) ||
			match.Score < -1 || match.Score > 1 {
			return nil, querybase.NewPublicationIncompleteFailure(
				fmt.Errorf("DRIFT Report vector match %d is invalid", index),
			)
		}
		if _, duplicate := seen[match.ReportID]; duplicate {
			return nil, querybase.NewPublicationIncompleteFailure(
				fmt.Errorf("DRIFT Report vector match %q occurs more than once", match.ReportID),
			)
		}
		report, exists := reports[match.ReportID]
		if !exists {
			return nil, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
				"DRIFT Report vector match %q is absent from ReportSet %q",
				match.ReportID,
				view.ReportSetID,
			))
		}
		seen[match.ReportID] = struct{}{}
		result[index] = SelectedReport{Report: report, Score: match.Score}
	}
	return result, nil
}

func (p *Primer) preflightReports(
	ctx context.Context,
	selected querybase.Epoch,
) (dimension int, resultErr error) {
	reader, err := p.vectors.OpenReports(ctx, selected.ReportSetID)
	if err != nil {
		return 0, normalizePublicationFailure(err)
	}
	if reader == nil {
		return 0, querybase.NewPublicationIncompleteFailure(
			errors.New("DRIFT Report vector Namespace is missing"),
		)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				querybase.NewInternalFailure(closeErr),
			)
		}
	}()
	if reader.ReportSetID() != selected.ReportSetID ||
		strings.TrimSpace(reader.Model()) == "" || reader.Model() != p.embedder.Model() ||
		reader.Dimension() <= 0 {
		return 0, querybase.NewPublicationIncompleteFailure(
			errors.New("DRIFT Report vector Namespace does not match the fixed ReportSet and embedding model"),
		)
	}
	return reader.Dimension(), nil
}
