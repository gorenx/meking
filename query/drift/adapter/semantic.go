// Package adapter maps provider contexts into DRIFT-owned Query contracts.
package adapter

import (
	"context"
	"errors"

	communityreport "github.com/memoria-space/meking/community/report"
	reportvector "github.com/memoria-space/meking/community/report/vectorindex"
	querybase "github.com/memoria-space/meking/query"
	querydrift "github.com/memoria-space/meking/query/drift"
	"github.com/memoria-space/meking/semantic"
)

type ReportVectorStore struct {
	store namespaceOpener
}

type namespaceOpener interface {
	Open(ctx context.Context, namespace semantic.Namespace) (semantic.NamespaceReader, error)
}

var _ querydrift.ReportVectorStore = (*ReportVectorStore)(nil)

func NewReportVectorStore(store namespaceOpener) (*ReportVectorStore, error) {
	if store == nil {
		return nil, errors.New("create DRIFT ReportVectorStore: Semantic store is required")
	}
	return &ReportVectorStore{store: store}, nil
}

func (s *ReportVectorStore) OpenReports(
	ctx context.Context,
	reportSetID string,
) (querydrift.ReportVectorReader, error) {
	if s == nil || s.store == nil {
		return nil, querybase.NewInternalFailure(
			errors.New("DRIFT ReportVectorStore is not configured"),
		)
	}
	name, err := reportvector.Namespace(communityreport.ReportSetID(reportSetID))
	if err != nil {
		return nil, querybase.NewPublicationIncompleteFailure(err)
	}
	reader, err := s.store.Open(ctx, semantic.Namespace(name))
	if err != nil {
		return nil, reportOpenFailure(err)
	}
	info, err := reader.Info(ctx)
	if err != nil {
		return nil, errors.Join(reportOpenFailure(err), reader.Close())
	}
	return &reportVectorReader{reader: reader, info: info}, nil
}

type reportVectorReader struct {
	reader semantic.NamespaceReader
	info   semantic.Info
}

func (r *reportVectorReader) ReportSetID() string { return string(r.info.Namespace) }
func (r *reportVectorReader) Model() string       { return r.info.Model }
func (r *reportVectorReader) Dimension() int      { return r.info.Dimension }

func (r *reportVectorReader) Search(
	ctx context.Context,
	vector []float64,
	limit int,
	reportIDs []string,
) ([]querydrift.ReportMatch, error) {
	if r == nil || r.reader == nil {
		return nil, querybase.NewInternalFailure(
			errors.New("DRIFT ReportVectorReader is not configured"),
		)
	}
	searchLimit := limit

	matches, err := r.reader.Search(ctx, vector, searchLimit, nil)
	if err != nil {
		return nil, reportReadFailure(err)
	}
	eligible := make(map[string]struct{}, len(reportIDs))
	for _, id := range reportIDs {
		eligible[id] = struct{}{}
	}
	result := make([]querydrift.ReportMatch, 0, min(limit, len(matches)))
	for _, match := range matches {
		if len(eligible) > 0 {
			if _, ok := eligible[match.ID]; !ok {
				continue
			}
		}
		result = append(result, querydrift.ReportMatch{ReportID: match.ID, Score: match.Score})
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (r *reportVectorReader) Close() error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

func reportOpenFailure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	return querybase.NewPublicationIncompleteFailure(err)
}

func reportReadFailure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return querybase.NewCancelledFailure(err)
	}
	return querybase.NewInternalFailure(err)
}
