package report

import (
	"context"
	"errors"
	"fmt"
	"strings"

	querybase "github.com/memoria-space/meking/query"
)

// ReportCatalog browses an immutable ReportSet selected by one Current or
// explicit historical Epoch. ReportSet creation owns source closure, so
// browsing does not reopen Knowledge or Corpus.
type ReportCatalog struct {
	epochs EpochReader
	// reports loads the exact ReportSet named by the fixed Epoch.
	reports *Reader
}

type EpochReader interface {
	querybase.EpochReader
	Epoch(ctx context.Context, id int64) (querybase.Epoch, error)
}

// NewReportCatalog constructs report browse and detail use cases from the
// unified Epoch selection and Community-owned immutable publication reader.
func NewReportCatalog(
	epochs EpochReader,
	publications ReportPublicationReader,
) (*ReportCatalog, error) {
	if epochs == nil {
		return nil, errors.New("create Report Catalog: Epoch reader is required")
	}
	if publications == nil {
		return nil, errors.New("create Report Catalog: publication reader is required")
	}
	reports, err := NewReader(publications)
	if err != nil {
		return nil, err
	}
	return &ReportCatalog{epochs: epochs, reports: reports}, nil
}

// Reader loads immutable report views by an Epoch already selected by the
// calling aggregate. It owns no Current selection and is safe to reuse inside
// Local, Global, and future DRIFT requests that pin Epoch at their boundary.
type Reader struct {
	publications ReportPublicationReader
}

// NewReader creates exact report reads from Community's immutable publication
// capability without adding another mutable selection.
func NewReader(publications ReportPublicationReader) (*Reader, error) {
	if publications == nil {
		return nil, errors.New("create Report Reader: publication reader is required")
	}
	return &Reader{publications: publications}, nil
}

// Browse fixes Current Epoch once and pages its ReportSet's deterministic
// Community order without consulting later Knowledge state.
func (c *ReportCatalog) Browse(
	ctx context.Context,
	request ReportPageRequest,
) (ReportPage, error) {
	if c == nil {
		return ReportPage{}, errors.New("Report Catalog is required")
	}
	if request.Page <= 0 || request.PageSize <= 0 || request.PageSize > MaximumReportPageSize {
		return ReportPage{}, fmt.Errorf(
			"%w: page and page size must be positive and page size must not exceed %d",
			ErrInvalidReportRequest,
			MaximumReportPageSize,
		)
	}
	view, err := c.Current(ctx)
	if err != nil {
		return ReportPage{}, err
	}
	pageReports := make([]ReportSummary, 0)
	pageIndex := request.Page - 1
	if len(view.Reports) > 0 && pageIndex <= (len(view.Reports)-1)/request.PageSize {
		start := pageIndex * request.PageSize
		end := start + min(request.PageSize, len(view.Reports)-start)
		pageReports = make([]ReportSummary, 0, end-start)
		for index := start; index < end; index++ {
			pageReports = append(pageReports, reportSummary(view.Reports[index], view.Communities[index]))
		}
	}
	return ReportPage{
		EpochID:     view.EpochID,
		ReportSetID: view.ReportSetID, CommunitySetID: view.CommunitySetID,
		CorporaID: view.CorporaID, Page: request.Page, PageSize: request.PageSize,
		Total: len(view.Reports), Reports: pageReports,
	}, nil
}

// Read returns one immutable Report from the exact Epoch selected by its list.
func (c *ReportCatalog) Read(
	ctx context.Context,
	reportID string,
	epochID int64,
) (ReportDetail, error) {
	if c == nil {
		return ReportDetail{}, errors.New("Report Catalog is required")
	}
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return ReportDetail{}, fmt.Errorf("%w: Report ID is required", ErrInvalidReportRequest)
	}
	if epochID <= 0 {
		return ReportDetail{}, fmt.Errorf("%w: Epoch ID must be positive", ErrInvalidReportRequest)
	}
	selected, err := c.epochs.Epoch(ctx, epochID)
	if err != nil {
		return ReportDetail{}, err
	}
	view, err := c.Publication(ctx, selected)
	if err != nil {
		return ReportDetail{}, err
	}
	index, found := reportPosition(view.Reports, reportID)
	if !found {
		return ReportDetail{}, ErrReportNotFound
	}
	return reportDetail(view, view.Reports[index], view.Communities[index]), nil
}

// Current fixes one Epoch and isolates its complete ReportSet for one Query use
// case without consulting later Knowledge or Corpus state.
func (c *ReportCatalog) Current(
	ctx context.Context,
) (View, error) {
	if c == nil {
		return View{}, errors.New("Report Catalog is required")
	}
	selected, err := c.epochs.Current(ctx)
	if errors.Is(err, querybase.ErrNoEpoch) {
		return View{}, ErrNoReportPublication
	}
	if err != nil {
		return View{}, err
	}
	return c.Publication(ctx, selected)
}

// Publication loads the exact immutable ReportSet named by an already-fixed
// Query Epoch. Local and Global use it without resolving Current a second time.
func (c *ReportCatalog) Publication(
	ctx context.Context,
	selected querybase.Epoch,
) (View, error) {
	if c == nil || c.reports == nil {
		return View{}, errors.New("Report Catalog is required")
	}
	return c.reports.Publication(ctx, selected)
}

// Publication loads the exact immutable ReportSet named by selected without
// consulting any Current pointer.
func (r *Reader) Publication(
	ctx context.Context,
	selected querybase.Epoch,
) (View, error) {
	if r == nil || r.publications == nil {
		return View{}, errors.New("Report Reader is required")
	}
	if selected.ReportSetID == "" {
		return View{}, ErrNoReportPublication
	}
	publication, err := r.publications.ReportPublication(ctx, selected.ReportSetID)
	if err != nil {
		return View{}, err
	}
	return View{
		EpochID:     selected.ID,
		ReportSetID: publication.ReportSetID, CommunitySetID: publication.CommunitySetID,
		CorporaID:   publication.CorporaID,
		Communities: copyPublishedCommunities(publication.Communities),
		Reports:     copyPublishedReports(publication.Reports),
	}, nil
}
