package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/community/adapter/sqlite/internal/db"
	communityreport "github.com/memoria-space/meking/community/report"
)

// SaveReportSet persists one complete immutable candidate without selecting it
// for Query. It joins the transaction carried by ctx so the caller can commit
// Reports, the candidate, its Publication state, and the next event together.
func (s *Store) SaveReportSet(
	ctx context.Context,
	set communityreport.ReportSet,
) error {
	if err := communityreport.ValidateReportSet(set); err != nil {
		return err
	}
	return s.write(ctx, func(statements statements) error {
		if _, err := statements.GetReportSet(ctx, string(set.ID)); err == nil {
			return fmt.Errorf(
				"%w: ReportSet ID %q already exists",
				communityreport.ErrReportSetConflict,
				set.ID,
			)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return classifySQLite("read existing ReportSet", err)
		}
		if _, err := statements.GetCommunitySet(ctx, string(set.CommunitySetID)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf(
					"%w: CommunitySet %q is not stored",
					communityreport.ErrInvalidReportSet,
					set.CommunitySetID,
				)
			}
			return classifySQLite("read ReportSet CommunitySet", err)
		}
		communities, err := statements.ListCommunities(ctx, string(set.CommunitySetID))
		if err != nil {
			return classifySQLite("list ReportSet Communities", err)
		}
		if len(communities) != len(set.Reports) {
			return fmt.Errorf(
				"%w: stored CommunitySet has %d Communities but ReportSet has %d Reports",
				communityreport.ErrInvalidReportSet,
				len(communities),
				len(set.Reports),
			)
		}
		storedCommunities := make(map[community.CommunityID]struct{}, len(communities))
		for _, current := range communities {
			storedCommunities[community.CommunityID(current.CommunityID)] = struct{}{}
		}
		communityIDs := make([]community.CommunityID, 0, len(set.Reports))
		for communityID := range set.Reports {
			communityIDs = append(communityIDs, communityID)
		}
		sort.Slice(communityIDs, func(left, right int) bool {
			return communityIDs[left] < communityIDs[right]
		})
		for _, communityID := range communityIDs {
			if _, found := storedCommunities[communityID]; !found {
				return fmt.Errorf(
					"%w: Community %q is outside stored CommunitySet %q",
					communityreport.ErrInvalidReportSet,
					communityID,
					set.CommunitySetID,
				)
			}
			reportID := set.Reports[communityID]
			reportRow, err := statements.GetReport(ctx, string(reportID))
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf(
						"%w: Report %q is not stored",
						communityreport.ErrInvalidReportSet,
						reportID,
					)
				}
				return classifySQLite("read ReportSet Report", err)
			}
			if reportRow.CommunityID != string(communityID) {
				return fmt.Errorf(
					"%w: Report %q belongs to Community %q, not %q",
					communityreport.ErrInvalidReportSet,
					reportID,
					reportRow.CommunityID,
					communityID,
				)
			}
		}
		if err := statements.CreateReportSet(ctx, db.CreateReportSetParams{
			ID:             string(set.ID),
			CommunitySetID: string(set.CommunitySetID),
			CorporaID:      set.CorporaID,
			CreatedAt:      set.CreatedAt.Format(time.RFC3339Nano),
		}); err != nil {
			return classifySQLite("create ReportSet", err)
		}
		for _, communityID := range communityIDs {
			if err := statements.AddReportSetReport(ctx, db.AddReportSetReportParams{
				ReportSetID: string(set.ID),
				CommunityID: string(communityID),
				ReportID:    string(set.Reports[communityID]),
			}); err != nil {
				return classifySQLite("add ReportSet Report", err)
			}
		}
		return nil
	})
}
