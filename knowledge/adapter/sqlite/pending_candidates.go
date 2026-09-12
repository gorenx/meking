package sqlite

import (
	"context"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
)

func (candidates *Candidates) ListEntityIDs(
	ctx context.Context,
	afterEntityID knowledge.EntityID,
	limit int,
) ([]knowledge.EntityID, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.BrowsePendingEntityIDs(ctx, db.BrowsePendingEntityIDsParams{
		ZoneID:    queries.zoneID,
		AfterID:   string(afterEntityID),
		PageLimit: int64(limit),
	})
	if err != nil {
		return nil, classifySQLite("browse pending Entity IDs", err)
	}
	result := make([]knowledge.EntityID, len(rows))
	for index, entityID := range rows {
		result[index] = knowledge.EntityID(entityID)
	}
	return result, nil
}

func (candidates *Candidates) ListRelationIDs(
	ctx context.Context,
	afterRelationID knowledge.RelationID,
	limit int,
) ([]knowledge.RelationID, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.BrowsePendingRelationIDs(ctx, db.BrowsePendingRelationIDsParams{
		ZoneID:    queries.zoneID,
		AfterID:   string(afterRelationID),
		PageLimit: int64(limit),
	})
	if err != nil {
		return nil, classifySQLite("browse pending Relation IDs", err)
	}
	result := make([]knowledge.RelationID, len(rows))
	for index, relationID := range rows {
		result[index] = knowledge.RelationID(relationID)
	}
	return result, nil
}

func (candidates *Candidates) ListClaimIDs(
	ctx context.Context,
	afterClaimID knowledge.ClaimID,
	limit int,
) ([]knowledge.ClaimID, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.BrowsePendingClaimIDs(ctx, db.BrowsePendingClaimIDsParams{
		ZoneID:    queries.zoneID,
		AfterID:   string(afterClaimID),
		PageLimit: int64(limit),
	})
	if err != nil {
		return nil, classifySQLite("browse pending Claim IDs", err)
	}
	result := make([]knowledge.ClaimID, len(rows))
	for index, claimID := range rows {
		result[index] = knowledge.ClaimID(claimID)
	}
	return result, nil
}
