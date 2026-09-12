// Package vectorindex owns the Report texts indexed for one immutable ReportSet.
package vectorindex

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	communityreport "github.com/memoria-space/meking/community/report"
)

const PageSize = 128

var ErrNotFound = errors.New("Report vector Namespace not found")

type Source struct {
	ID   string
	Text string
}

func Namespace(reportSetID communityreport.ReportSetID) (string, error) {
	if strings.TrimSpace(string(reportSetID)) == "" ||
		strings.TrimSpace(string(reportSetID)) != string(reportSetID) {
		return "", errors.New("Report vector ReportSetID is invalid")
	}
	return string(reportSetID), nil
}

func Sources(
	set communityreport.ReportSet,
	reports []communityreport.Report,
) ([]Source, error) {
	if err := communityreport.ValidateReportSet(set); err != nil {
		return nil, err
	}
	byID := make(map[communityreport.ID]communityreport.Report, len(reports))
	for _, report := range reports {
		if err := communityreport.ValidateID(report.ID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(report.FullContent) == "" {
			return nil, fmt.Errorf("Report %q has empty FullContent", report.ID)
		}
		if _, duplicate := byID[report.ID]; duplicate {
			return nil, fmt.Errorf("Report %q is duplicated", report.ID)
		}
		byID[report.ID] = report
	}
	if len(byID) != len(set.Reports) {
		return nil, fmt.Errorf(
			"ReportSet %q selects %d Reports but %d were supplied",
			set.ID, len(set.Reports), len(byID),
		)
	}
	ids := make([]communityreport.ID, 0, len(set.Reports))
	for _, id := range set.Reports {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	result := make([]Source, len(ids))
	for index, id := range ids {
		report, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("ReportSet %q selects missing Report %q", set.ID, id)
		}
		result[index] = Source{ID: string(id), Text: report.FullContent}
	}
	return result, nil
}
