package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/community/adapter/sqlite/internal/db"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/zone"
)

var _ communityreport.PublicationStore = (*Store)(nil)

func (store *Store) LoadReportPublication(
	ctx context.Context,
	structureID community.StructureID,
) (communityreport.Publication, error) {
	if _, err := community.RestoreStructureID(structureID); err != nil {
		return communityreport.Publication{}, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return communityreport.Publication{}, err
	}
	row, err := newGlobalStatements(store.database).GetReportPublication(
		ctx,
		db.GetReportPublicationParams{
			ZoneID:      string(zoneID),
			StructureID: string(structureID),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return communityreport.Publication{}, communityreport.ErrPublicationNotFound
	}
	if err != nil {
		return communityreport.Publication{}, classifySQLite("read Community Report publication", err)
	}
	return restoreReportPublication(row)
}

func (store *Store) PrepareReportPublication(
	ctx context.Context,
	publication communityreport.Publication,
) error {
	if err := publication.Validate(); err != nil {
		return err
	}
	if publication.VectorsReady {
		return communityreport.ErrPublicationConflict
	}
	return store.write(ctx, func(statements statements) error {
		inserted, err := statements.queries.CreateReportPublication(
			ctx,
			db.CreateReportPublicationParams{
				ZoneID:      statements.zoneID,
				StructureID: string(publication.StructureID),
				EpochID:     publication.EpochID,
				ReportSetID: string(publication.ReportSetID),
				CreatedAt:   publication.CreatedAt.Format(time.RFC3339Nano),
			},
		)
		if err != nil {
			return classifySQLite("prepare Community Report publication", err)
		}
		if inserted == 1 {
			return nil
		}
		row, err := statements.queries.GetReportPublication(
			ctx,
			db.GetReportPublicationParams{
				ZoneID:      statements.zoneID,
				StructureID: string(publication.StructureID),
			},
		)
		if err != nil {
			return communityreport.ErrPublicationConflict
		}
		existing, err := restoreReportPublication(row)
		if err != nil || existing != publication {
			return communityreport.ErrPublicationConflict
		}
		return nil
	})
}

func (store *Store) CompleteReportPublication(
	ctx context.Context,
	structureID community.StructureID,
	reportSetID communityreport.ReportSetID,
) error {
	if _, err := community.RestoreStructureID(structureID); err != nil {
		return err
	}
	if err := communityreport.ValidateReportSetID(reportSetID); err != nil {
		return err
	}
	return store.write(ctx, func(statements statements) error {
		updated, err := statements.queries.CompleteReportPublication(
			ctx,
			db.CompleteReportPublicationParams{
				ZoneID:      statements.zoneID,
				StructureID: string(structureID),
				ReportSetID: string(reportSetID),
			},
		)
		if err != nil {
			return classifySQLite("complete Community Report publication", err)
		}
		if updated == 1 {
			return nil
		}
		row, err := statements.queries.GetReportPublication(
			ctx,
			db.GetReportPublicationParams{
				ZoneID:      statements.zoneID,
				StructureID: string(structureID),
			},
		)
		if err != nil {
			return communityreport.ErrPublicationConflict
		}
		existing, err := restoreReportPublication(row)
		if err != nil || existing.ReportSetID != reportSetID || !existing.VectorsReady {
			return communityreport.ErrPublicationConflict
		}
		return nil
	})
}

func restoreReportPublication(
	row db.GetReportPublicationRow,
) (communityreport.Publication, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
	if err != nil || row.VectorsReady < 0 || row.VectorsReady > 1 {
		return communityreport.Publication{}, community.ErrCommunityDataIntegrity
	}
	publication := communityreport.Publication{
		EpochID:      row.EpochID,
		StructureID:  community.StructureID(row.StructureID),
		ReportSetID:  communityreport.ReportSetID(row.ReportSetID),
		VectorsReady: row.VectorsReady == 1,
		CreatedAt:    createdAt,
	}
	if err := publication.Validate(); err != nil {
		return communityreport.Publication{}, fmt.Errorf(
			"%w: %v",
			community.ErrCommunityDataIntegrity,
			err,
		)
	}
	return publication, nil
}
