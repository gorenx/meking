package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/zone"
)

// LoadReportSet reconstructs one exact immutable ReportSet in a deferred read
// transaction and verifies every stored Community and Report reference.
func (s *Store) LoadReportSet(
	ctx context.Context,
	id communityreport.ReportSetID,
) (communityreport.ReportSet, error) {
	if err := communityreport.ValidateReportSetID(id); err != nil {
		return communityreport.ReportSet{}, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return communityreport.ReportSet{}, fmt.Errorf("read ReportSet: %w", err)
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return communityreport.ReportSet{}, classifySQLite("begin ReportSet read", err)
	}
	defer transaction.Rollback()
	return loadReportSet(ctx, newStatements(transaction, string(zoneID)), id)
}

func loadReportSet(
	ctx context.Context,
	statements statements,
	id communityreport.ReportSetID,
) (communityreport.ReportSet, error) {
	row, err := statements.GetReportSet(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return communityreport.ReportSet{}, communityreport.ErrReportSetNotFound
	}
	if err != nil {
		return communityreport.ReportSet{}, classifySQLite("read ReportSet", err)
	}
	if _, err := statements.GetCommunitySet(ctx, row.CommunitySetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return communityreport.ReportSet{}, fmt.Errorf(
				"%w: ReportSet %q references missing CommunitySet %q",
				community.ErrCommunityDataIntegrity,
				id,
				row.CommunitySetID,
			)
		}
		return communityreport.ReportSet{}, classifySQLite("read ReportSet CommunitySet", err)
	}
	communities, err := statements.ListCommunities(ctx, row.CommunitySetID)
	if err != nil {
		return communityreport.ReportSet{}, classifySQLite("list ReportSet Communities", err)
	}
	storedCommunities := make(map[string]struct{}, len(communities))
	for _, current := range communities {
		storedCommunities[current.CommunityID] = struct{}{}
	}
	mappings, err := statements.ListReportSetReports(ctx, string(id))
	if err != nil {
		return communityreport.ReportSet{}, classifySQLite("list ReportSet Reports", err)
	}
	if len(mappings) != len(communities) {
		return communityreport.ReportSet{}, fmt.Errorf(
			"%w: ReportSet %q has %d mappings for %d Communities",
			community.ErrCommunityDataIntegrity,
			id,
			len(mappings),
			len(communities),
		)
	}
	reports := make(map[community.CommunityID]communityreport.ID, len(mappings))
	for _, mapping := range mappings {
		if _, found := storedCommunities[mapping.CommunityID]; !found {
			return communityreport.ReportSet{}, fmt.Errorf(
				"%w: ReportSet %q Community %q is outside CommunitySet %q",
				community.ErrCommunityDataIntegrity,
				id,
				mapping.CommunityID,
				row.CommunitySetID,
			)
		}
		reportRow, err := statements.GetReport(ctx, mapping.ReportID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return communityreport.ReportSet{}, fmt.Errorf(
					"%w: ReportSet %q references missing Report %q",
					community.ErrCommunityDataIntegrity,
					id,
					mapping.ReportID,
				)
			}
			return communityreport.ReportSet{}, classifySQLite("read ReportSet Report", err)
		}
		if reportRow.CommunityID != mapping.CommunityID {
			return communityreport.ReportSet{}, fmt.Errorf(
				"%w: ReportSet %q maps Report %q to the wrong Community",
				community.ErrCommunityDataIntegrity,
				id,
				mapping.ReportID,
			)
		}
		reports[community.CommunityID(mapping.CommunityID)] =
			communityreport.ID(mapping.ReportID)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
	if err != nil {
		return communityreport.ReportSet{}, fmt.Errorf(
			"%w: ReportSet %q has invalid CreatedAt: %v",
			community.ErrCommunityDataIntegrity,
			id,
			err,
		)
	}
	set := communityreport.ReportSet{
		ID:             communityreport.ReportSetID(row.ID),
		CommunitySetID: community.CommunitySetID(row.CommunitySetID),
		CorporaID:      row.CorporaID,
		Reports:        reports,
		CreatedAt:      createdAt,
	}
	if err := communityreport.ValidateReportSet(set); err != nil {
		return communityreport.ReportSet{}, fmt.Errorf(
			"%w: ReportSet %q is invalid: %v",
			community.ErrCommunityDataIntegrity,
			id,
			err,
		)
	}
	return set, nil
}
