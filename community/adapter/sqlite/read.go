package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/community/adapter/sqlite/internal/db"
	"math"
	"time"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/zone"
)

const communitySetTimeFormat = time.RFC3339Nano

// Load reconstructs and validates one exact immutable CommunitySet in a
// deferred read transaction.
func (s *Store) Load(
	ctx context.Context,
	id community.CommunitySetID,
) (community.CommunitySet, error) {
	if err := community.ValidateCommunitySetID(id); err != nil {
		return community.CommunitySet{}, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return community.CommunitySet{}, fmt.Errorf("read CommunitySet: %w", err)
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return community.CommunitySet{}, classifySQLite("begin CommunitySet read", err)
	}
	defer transaction.Rollback()
	return loadCommunitySet(ctx, newStatements(transaction, string(zoneID)), id)
}

func loadCommunitySet(
	ctx context.Context,
	statements statements,
	id community.CommunitySetID,
) (community.CommunitySet, error) {
	row, err := statements.GetCommunitySet(ctx, string(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return community.CommunitySet{}, community.ErrCommunitySetNotFound
		}
		return community.CommunitySet{}, classifySQLite("read CommunitySet", err)
	}
	if value, err := statements.FindCommunityWithoutParent(ctx, string(id)); err == nil {
		return community.CommunitySet{}, fmt.Errorf(
			"%w: CommunitySet %q Community %q has no parent row",
			community.ErrCommunityDataIntegrity,
			id,
			value,
		)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return community.CommunitySet{}, classifySQLite("check Community parent references", err)
	}
	if value, err := statements.FindMemberWithoutCommunity(ctx, string(id)); err == nil {
		return community.CommunitySet{}, fmt.Errorf(
			"%w: CommunitySet %q Entity %q has no Community row",
			community.ErrCommunityDataIntegrity,
			id,
			value,
		)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return community.CommunitySet{}, classifySQLite("check Community member owner", err)
	}
	if value, err := statements.FindMemberWithoutEntity(ctx, string(id)); err == nil {
		return community.CommunitySet{}, fmt.Errorf(
			"%w: CommunitySet %q member Entity %q has no version reference",
			community.ErrCommunityDataIntegrity,
			id,
			value,
		)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return community.CommunitySet{}, classifySQLite("check Community member Entity", err)
	}

	createdAt, err := time.Parse(communitySetTimeFormat, row.CreatedAt)
	if err != nil {
		return community.CommunitySet{}, fmt.Errorf(
			"%w: CommunitySet %q has invalid CreatedAt: %v",
			community.ErrCommunityDataIntegrity,
			id,
			err,
		)
	}
	if row.DetectorVersion <= 0 || row.DetectorVersion > math.MaxUint32 {
		return community.CommunitySet{}, fmt.Errorf(
			"%w: CommunitySet %q has invalid DetectorVersion %d",
			community.ErrCommunityDataIntegrity,
			id,
			row.DetectorVersion,
		)
	}
	if row.MaxClusterSize <= 0 ||
		uint64(row.MaxClusterSize) > uint64(^uint(0)>>1) {
		return community.CommunitySet{}, fmt.Errorf(
			"%w: CommunitySet %q has invalid MaxClusterSize %d",
			community.ErrCommunityDataIntegrity,
			id,
			row.MaxClusterSize,
		)
	}
	useLargestConnectedComponent, err := storedBool(
		row.UseLargestConnectedComponent,
		"UseLargestConnectedComponent",
	)
	if err != nil {
		return community.CommunitySet{}, err
	}
	entityRows, err := statements.ListCommunityEntities(ctx, string(id))
	if err != nil {
		return community.CommunitySet{}, classifySQLite("list CommunitySet Entities", err)
	}
	entities := make([]community.EntityReference, len(entityRows))
	for index, entity := range entityRows {
		entities[index] = community.EntityReference{
			ID:      entity.EntityID,
			Version: uint64(entity.Version),
		}
	}
	relationRows, err := statements.ListCommunityRelations(ctx, string(id))
	if err != nil {
		return community.CommunitySet{}, classifySQLite("list CommunitySet Relations", err)
	}
	relations := make([]community.RelationReference, len(relationRows))
	for index, relation := range relationRows {
		relations[index] = community.RelationReference{
			ID:      relation.RelationID,
			Version: uint64(relation.Version),
		}
	}
	communityRows, err := statements.ListCommunities(ctx, string(id))
	if err != nil {
		return community.CommunitySet{}, classifySQLite("list Communities", err)
	}
	communities := make([]community.Membership, len(communityRows))
	for index, current := range communityRows {
		members, err := statements.ListCommunityMembers(
			ctx,
			db.ListCommunityMembersParams{
				CommunitySetID: string(id),
				CommunityID:    current.CommunityID,
			},
		)
		if err != nil {
			return community.CommunitySet{}, classifySQLite("list Community members", err)
		}
		entityIDs := make([]string, len(members))
		for ordinal, member := range members {
			if member.MemberOrdinal != int64(ordinal) {
				return community.CommunitySet{}, fmt.Errorf(
					"%w: CommunitySet %q Community %q has non-contiguous members",
					community.ErrCommunityDataIntegrity,
					id,
					current.CommunityID,
				)
			}
			entityIDs[ordinal] = member.EntityID
		}
		var parentID *community.CommunityID
		if current.ParentCommunityID.Valid {
			value := community.CommunityID(current.ParentCommunityID.String)
			parentID = &value
		}
		final, err := storedBool(current.Final, "Final")
		if err != nil {
			return community.CommunitySet{}, err
		}
		unsplittable, err := storedBool(current.Unsplittable, "Unsplittable")
		if err != nil {
			return community.CommunitySet{}, err
		}
		communities[index] = community.Membership{
			ID:           community.CommunityID(current.CommunityID),
			Number:       int(current.Number),
			Level:        int(current.Level),
			ParentID:     parentID,
			Final:        final,
			Unsplittable: unsplittable,
			EntityIDs:    entityIDs,
		}
	}
	set := community.CommunitySet{
		ID:              community.CommunitySetID(row.ID),
		DetectorVersion: uint32(row.DetectorVersion),
		DetectionConfig: community.DetectConfig{
			MaxClusterSize:               int(row.MaxClusterSize),
			UseLargestConnectedComponent: useLargestConnectedComponent,
			Seed:                         row.Seed,
		},
		Communities: communities,
		Entities:    entities,
		Relations:   relations,
		CreatedAt:   createdAt,
	}
	if err := community.ValidateCommunitySet(set); err != nil {
		return community.CommunitySet{}, fmt.Errorf(
			"%w: %v",
			community.ErrCommunityDataIntegrity,
			err,
		)
	}
	return set, nil
}

func storedBool(value int64, field string) (bool, error) {
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf(
			"%w: Community %s has invalid boolean %d",
			community.ErrCommunityDataIntegrity,
			field,
			value,
		)
	}
}
