// Package adapter maps cross-context publication identities into the shared
// Query contracts used by Basic, Local, Global, DRIFT, and report browsing.
package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/epoch"
	querybase "github.com/memoria-space/meking/query"
)

// Epochs is the provider capability required to fix Current or one exact Epoch.
// The provider validates its own persisted value before this adapter projects it.
type Epochs interface {
	Current(ctx context.Context) (epoch.Epoch, error)
	Epoch(ctx context.Context, id epoch.ID) (epoch.Epoch, error)
}

type ReportPublications interface {
	LoadReportPublication(context.Context, community.StructureID) (communityreport.Publication, error)
}

// EpochReader projects Epoch's publication manifest into Query's minimal
// shared identity contract without exposing provider persistence or timestamps.
type EpochReader struct {
	epochs       Epochs
	publications ReportPublications
}

// NewEpochReader creates the Query adapter for one already-open Epoch reader.
func NewEpochReader(
	epochs Epochs,
	publications ReportPublications,
) (*EpochReader, error) {
	if epochs == nil {
		return nil, errors.New("create Query Epoch reader: Epochs are required")
	}
	if publications == nil {
		return nil, errors.New("create Query Epoch reader: ReportPublications are required")
	}
	return &EpochReader{
		epochs:       epochs,
		publications: publications,
	}, nil
}

// Current fixes one provider Epoch and copies only Query-consumed identities.
func (r *EpochReader) Current(ctx context.Context) (querybase.Epoch, error) {
	if r == nil || r.epochs == nil {
		return querybase.Epoch{}, errors.New("Query Epoch reader is not configured")
	}
	current, err := r.epochs.Current(ctx)
	if errors.Is(err, epoch.ErrEpochNotFound) {
		return querybase.Epoch{}, querybase.ErrNoEpoch
	}
	if err != nil {
		return querybase.Epoch{}, err
	}
	return r.projectEpoch(ctx, current)
}

// Epoch projects one exact immutable publication without consulting Current.
func (r *EpochReader) Epoch(ctx context.Context, id int64) (querybase.Epoch, error) {
	if r == nil || r.epochs == nil {
		return querybase.Epoch{}, errors.New("Query Epoch reader is not configured")
	}
	value, err := r.epochs.Epoch(ctx, epoch.ID(id))
	if errors.Is(err, epoch.ErrEpochNotFound) {
		return querybase.Epoch{}, querybase.ErrEpochNotFound
	}
	if err != nil {
		return querybase.Epoch{}, err
	}
	return r.projectEpoch(ctx, value)
}

func (r *EpochReader) projectEpoch(
	ctx context.Context,
	value epoch.Epoch,
) (querybase.Epoch, error) {
	result := querybase.Epoch{
		ID:          int64(value.ID),
		Knowledge:   value.Knowledge.Clone(),
		CorporaID:   string(value.CorporaID),
		StructureID: string(value.StructureID),
	}
	structureID := community.StructureID(value.StructureID)
	publication, err := r.publications.LoadReportPublication(ctx, structureID)
	switch {
	case errors.Is(err, communityreport.ErrPublicationNotFound):
		return result, nil
	case err != nil:
		return querybase.Epoch{}, err
	case publication.EpochID != int64(value.ID) || publication.StructureID != structureID:
		return querybase.Epoch{}, communityreport.ErrPublicationConflict
	case !publication.VectorsReady:
		return result, nil
	default:
		result.ReportSetID = string(publication.ReportSetID)
		return result, nil
	}
}

var _ querybase.EpochReader = (*EpochReader)(nil)
